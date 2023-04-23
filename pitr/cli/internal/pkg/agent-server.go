/*
* Licensed to the Apache Software Foundation (ASF) under one or more
* contributor license agreements.  See the NOTICE file distributed with
* this work for additional information regarding copyright ownership.
* The ASF licenses this file to You under the Apache License, Version 2.0
* (the "License"); you may not use this file except in compliance with
* the License.  You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package pkg

import (
	"context"
	"fmt"
	"net/http"

	"github.com/apache/shardingsphere-on-cloud/pitr/cli/internal/pkg/model"
	"github.com/apache/shardingsphere-on-cloud/pitr/cli/internal/pkg/xerr"
	"github.com/apache/shardingsphere-on-cloud/pitr/cli/pkg/httputils"
	"github.com/google/uuid"
)

type agentServer struct {
	addr string

	_apiBackup     string
	_apiRestore    string
	_apiShowDetail string
	_apiShowList   string
}

type IAgentServer interface {
	CheckStatus() error
	Backup(in *model.BackupIn) (string, error)
	Restore(in *model.RestoreIn) error
	ShowDetail(in *model.ShowDetailIn) (*model.BackupInfo, error)
	ShowList(in *model.ShowListIn) ([]model.BackupInfo, error)
}

var _ IAgentServer = (*agentServer)(nil)

func NewAgentServer(addr string) IAgentServer {
	return &agentServer{
		addr: addr,

		_apiBackup:     "/api/backup",
		_apiRestore:    "/api/restore",
		_apiShowDetail: "/api/show",
		_apiShowList:   "/api/show/list",
	}
}

// CheckStatus check agent server is alive
func (as *agentServer) CheckStatus() error {
	url := fmt.Sprintf("%s/%s", as.addr, "ping")
	r := httputils.NewRequest(context.Background(), http.MethodGet, url)
	httpCode, err := r.Send(nil)
	if err != nil {
		efmt := "httputils.NewRequest[url=%s] return err=%s,wrap=%w"
		return fmt.Errorf(efmt, url, err, xerr.NewCliErr(xerr.Unknown))
	}
	if httpCode != http.StatusOK {
		return fmt.Errorf("httpCode=%d", httpCode)
	}
	return nil
}

func (as *agentServer) Backup(in *model.BackupIn) (string, error) {
	url := fmt.Sprintf("%s%s", as.addr, as._apiBackup)

	out := &model.BackupOutResp{}
	r := httputils.NewRequest(context.Background(), http.MethodPost, url)
	r.Header(map[string]string{
		"x-request-id": uuid.New().String(),
		"content-type": "application/json",
	})
	r.Body(in)

	httpCode, err := r.Send(out)
	if err != nil {
		efmt := "httputils.NewRequest[url=%s,body=%v,out=%v] return err=%s,wrap=%w"
		return "", fmt.Errorf(efmt, url, in, out, err, xerr.NewCliErr(xerr.Unknown))
	}

	if httpCode != http.StatusOK {
		return "", fmt.Errorf("unknown http status[code=%d],err=%w", httpCode, xerr.NewCliErr(xerr.InvalidHTTPStatus))
	}

	if out.Code != 0 {
		asErr := xerr.NewAgentServerErr(out.Code, out.Msg)
		return "", fmt.Errorf("agent server error[code=%d,msg=%s],err=%w", out.Code, out.Msg, asErr)
	}

	return out.Data.ID, nil
}

func (as *agentServer) Restore(in *model.RestoreIn) error {
	url := fmt.Sprintf("%s%s", as.addr, as._apiRestore)

	out := &model.RestoreResp{}
	r := httputils.NewRequest(context.Background(), http.MethodPost, url)
	r.Header(map[string]string{
		"x-request-id": uuid.New().String(),
		"content-type": "application/json",
	})
	r.Body(in)
	httpCode, err := r.Send(out)
	if err != nil {
		efmt := "httputils.NewRequest[url=%s,body=%v,out=%v] return err=%s,wrap=%w"
		return fmt.Errorf(efmt, url, in, out, err, xerr.NewCliErr(xerr.Unknown))
	}

	if httpCode != http.StatusOK {
		return fmt.Errorf("unknown http status[code=%d],err=%w", httpCode, xerr.NewCliErr(xerr.InvalidHTTPStatus))
	}

	if out.Code != 0 {
		asErr := xerr.NewAgentServerErr(out.Code, out.Msg)
		return fmt.Errorf("agent server error[code=%d,msg=%s],err=%w", out.Code, out.Msg, asErr)
	}

	return nil
}

func (as *agentServer) ShowDetail(in *model.ShowDetailIn) (*model.BackupInfo, error) {
	url := fmt.Sprintf("%s%s", as.addr, as._apiShowDetail)

	out := &model.BackupDetailResp{}
	r := httputils.NewRequest(context.Background(), http.MethodPost, url)
	r.Header(map[string]string{
		"x-request-id": uuid.New().String(),
		"content-type": "application/json",
	})
	r.Body(in)
	httpCode, err := r.Send(out)
	if err != nil {
		efmt := "httputils.NewRequest[url=%s,body=%v,out=%v] return err=%s,wrap=%w"
		return nil, fmt.Errorf(efmt, url, in, out, err, xerr.NewCliErr(xerr.Unknown))
	}

	if httpCode != http.StatusOK {
		return nil, fmt.Errorf("unknown http status[code=%d],err=%w", httpCode, xerr.NewCliErr(xerr.InvalidHTTPStatus))
	}

	if out.Code != 0 {
		asErr := xerr.NewAgentServerErr(out.Code, out.Msg)
		return nil, fmt.Errorf("agent server error[code=%d,msg=%s],err=%w", out.Code, out.Msg, asErr)
	}

	return &out.Data, nil
}

func (as *agentServer) ShowList(in *model.ShowListIn) ([]model.BackupInfo, error) {
	url := fmt.Sprintf("%s%s", as.addr, as._apiShowList)

	out := &model.BackupListResp{}
	r := httputils.NewRequest(context.Background(), http.MethodPost, url)
	r.Header(map[string]string{
		"x-request-id": uuid.New().String(),
		"content-type": "application/json",
	})
	r.Body(in)
	httpCode, err := r.Send(out)
	if err != nil {
		efmt := "httputils.NewRequest[url=%s,body=%v,out=%v] return err=%s,wrap=%w"
		return nil, fmt.Errorf(efmt, url, in, out, err, xerr.NewCliErr(xerr.Unknown))
	}

	if httpCode != http.StatusOK {
		return nil, fmt.Errorf("unknown http status[code=%d],err=%w", httpCode, xerr.NewCliErr(xerr.InvalidHTTPStatus))
	}

	if out.Code != 0 {
		asErr := xerr.NewAgentServerErr(out.Code, out.Msg)
		return nil, fmt.Errorf("agent server error[code=%d,msg=%s],err=%w", out.Code, out.Msg, asErr)
	}

	return out.Data, nil
}
