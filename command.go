/*
 * Copyright (C) 2017 Jianhui Zhao <jianhuizhao329@gmail.com>
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301
 * USA
 */

package main

import (
    "time"
    "sync"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "github.com/gorilla/websocket"
    "github.com/zhaojh329/rttys/rtty"
)

const (
    COMMAND_ERR_NONE = iota
    COMMAND_ERR_TIMEOUT
    COMMAND_ERR_NOTFOUND
    COMMAND_ERR_READ
    COMMAND_ERR_LOGIN
    COMMAND_ERR_SYS
    COMMAND_ERR_PARAMETER
    COMMAND_ERR_DEVOFFLINE
)

type CommandReq struct {
    ID uint32 `json:"id"`
    Username string `json:"username"`
    Password string `json:"password"`
    Devid string `json:"devid"`
    Cmd string `json:"cmd"`
    Params []string `json:"params"`
    Env map[string]string `json:"env"`
}

type CommandResult struct {
    ID uint32 `json:"id,omitempty"`
    Err int `json:err`
    Msg string `json:"msg"`
    Code int `json:"code"`
    Stdout string `json:"stdout"`
    Stderr string `json:"stderr"`
}

var errStr = map[int]string {
    COMMAND_ERR_NONE: "",
    COMMAND_ERR_TIMEOUT: "timeout",
    COMMAND_ERR_NOTFOUND: "not found",
    COMMAND_ERR_READ: "read error",
    COMMAND_ERR_LOGIN: "login failed",
    COMMAND_ERR_SYS: "system error",
    COMMAND_ERR_PARAMETER: "devid and cmd required",
    COMMAND_ERR_DEVOFFLINE: "device offline",
}

var commandID uint32 = 0
var cmdMutex sync.Mutex
var command = make(map[uint32]chan *rtty.RttyMessage)

func serveCmd(br *Broker, w http.ResponseWriter, r *http.Request) {
    ticker := time.NewTicker(time.Second * 5)
    defer func() {
        ticker.Stop()
    }()

    err := COMMAND_ERR_NONE

    body, _ := ioutil.ReadAll(r.Body)
    r.Body.Close()

    req := CommandReq{}
    json.Unmarshal(body, &req)

    if req.Devid == "" || req.Cmd == "" {
        err = COMMAND_ERR_PARAMETER
    } else if dev, ok := br.devices[req.Devid]; !ok {
        err = COMMAND_ERR_DEVOFFLINE
    } else {
        cmdMutex.Lock()
        id := commandID
        command[id] = make(chan *rtty.RttyMessage)
        commandID = commandID + 1
        if commandID == 1024 {
            commandID = 0
        }
        cmd := command[id]
        cmdMutex.Unlock()

        msg := RttyMessageInit(&rtty.RttyMessage{
            Version: RTTY_MESSAGE_VERSION,
            Type: rtty.RttyMessage_COMMAND,
            Id: id,
            Name: req.Cmd,
            Username: req.Username,
            Password: req.Password,
            Params: req.Params,
            Env: req.Env,
        })

        dev.wsWrite(websocket.BinaryMessage, msg)

        select {
        case msg := <- cmd:
            res := CommandResult{
                Stdout: msg.Stdout,
                Stderr: msg.Stderr,
            }

            cmdMutex.Lock()
            delete(command, msg.Id)
            cmdMutex.Unlock()

            js, _ := json.Marshal(res)
            w.Write(js)

            return
        case <- ticker.C:
            err = COMMAND_ERR_TIMEOUT
            goto Err
        }
    }

Err:
    res := CommandResult{Err: err, Msg: errStr[err]}
    js, _ := json.Marshal(res)
    w.Write(js)
}
