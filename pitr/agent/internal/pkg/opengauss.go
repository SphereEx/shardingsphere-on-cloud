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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/apache/shardingsphere-on-cloud/pitr/agent/pkg/gsutil"
	"github.com/apache/shardingsphere-on-cloud/pitr/agent/pkg/logging"
	"strings"

	"github.com/dlclark/regexp2"

	"github.com/apache/shardingsphere-on-cloud/pitr/agent/internal/cons"
	"github.com/apache/shardingsphere-on-cloud/pitr/agent/internal/pkg/model"
	"github.com/apache/shardingsphere-on-cloud/pitr/agent/pkg/cmds"
)

type (
	openGauss struct {
		shell      string
		pgData     string
		pgDataTemp string
		log        logging.ILog
	}

	IOpenGauss interface {
		AsyncBackup(backupPath, instanceName, backupMode string, threadsNum uint8) (string, error)
		ShowBackup(backupPath, instanceName, backupID string) (*model.Backup, error)
		Init(backupPath string) error
		AddInstance(backupPath, instance string) error
		DelInstance(backupPath, instance string) error
		Start() error
		Stop() error
		Status() (string, error)
		Restore(backupPath, instance, backupID string) error
		ShowBackupList(backupPath, instanceName string) ([]model.Backup, error)
		Auth(user, password, dbName string, dbPort uint16) error
		MvTempToPgData() error
		MvPgDataToTemp() error
		CleanPgDataTemp() error
	}
)

func NewOpenGauss(shell, pgData string, log logging.ILog) IOpenGauss {
	dirs := strings.Split(pgData, "/")
	dirs = append(dirs[0:len(dirs)-1], "temp")

	return &openGauss{
		shell:      shell,
		pgData:     pgData,
		pgDataTemp: strings.Join(dirs, "/"),
		log:        log,
	}
}

const (
	_backupFmt    = "gs_probackup backup --backup-path=%s --instance=%s --backup-mode=%s --pgdata=%s --threads=%d 2>&1"
	_showFmt      = "gs_probackup show --instance=%s --backup-path=%s --backup-id=%s --format=json 2>&1"
	_delBackupFmt = "gs_probackup delete --backup-path=%s --instance=%s --backup-id=%s 2>&1"
	_restoreFmt   = "gs_probackup restore --backup-path=%s --instance=%s --backup-id=%s --pgdata=%s 2>&1"

	_initFmt  = "gs_probackup init --backup-path=%s 2>&1"
	_rmDirFmt = "rm -r %s"

	_addInstanceFmt = "gs_probackup add-instance --backup-path=%s --instance=%s --pgdata=%s 2>&1"
	_delInstanceFmt = "gs_probackup del-instance --backup-path=%s --instance=%s 2>&1"

	_startOpenGaussFmt = "gs_ctl start --pgdata=%s"
	_stopOpenGaussFmt  = "gs_ctl stop --pgdata=%s"
	_statusGaussFmt    = "gs_ctl status --pgdata=%s"

	_showListFmt = "gs_probackup show --instance=%s --backup-path=%s --format=json 2>&1"

	_mvFmt = "mv %s %s"
)

func (og *openGauss) AsyncBackup(backupPath, instanceName, backupMode string, threadsNum uint8) (string, error) {
	cmd := fmt.Sprintf(_backupFmt, backupPath, instanceName, backupMode, og.pgData, threadsNum)
	outputs, err := cmds.AsyncExec(og.shell, cmd)
	if err != nil {
		return "", fmt.Errorf("cmds.AsyncExec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}

	for output := range outputs {
		og.log.
			Field("backup_path", backupPath).
			Field("instance", instanceName).
			Field("backup_mode", backupMode).
			Field("pgdata", og.pgData).
			Debug(fmt.Sprintf("AsyncBackup output[lineNo=%d,msg=%s,err=%v]", output.LineNo, output.Message, output.Error))

		if output.Error != nil {
			return "", fmt.Errorf("output.Error[%w] is not nil", output.Error)
		}

		// get the backup id from the first line
		bid, err := og.getBackupID(output.Message)
		if err != nil {
			return "", fmt.Errorf("og.getBackupID[source=%s] return err=%w", output.Message, err)
		}
		//ignore other output
		go og.ignore(outputs)
		return bid, nil
	}
	return "", fmt.Errorf("unknow err")
}

func (og *openGauss) ShowBackup(backupPath, instanceName, backupID string) (*model.Backup, error) {
	cmd := fmt.Sprintf(_showFmt, instanceName, backupPath, backupID)
	output, err := cmds.Exec(og.shell, cmd)
	if err != nil {
		return nil, fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}

	var list []model.BackupList
	if err = json.Unmarshal([]byte(output), &list); err != nil {
		return nil, fmt.Errorf("json.Unmarshal[output=%s] return err=%s,wrap=%w", output, err, cons.Internal)
	}

	for _, ins := range list {
		if ins.Instance == instanceName {
			if len(ins.List) == 0 {
				return nil, fmt.Errorf("instance[name=%s],backupList[v=%+v],err=%w", ins.Instance, list, cons.DataNotFound)
			}

			return &ins.List[0], nil
		}
	}

	return nil, fmt.Errorf("backupList[v=%+v],err=%w", list, cons.DataNotFound)
}

func (og *openGauss) delBackup(backupPath, instanceName, backupID string) error {
	cmd := fmt.Sprintf(_delBackupFmt, backupPath, instanceName, backupID)
	_, err := cmds.Exec(og.shell, cmd)
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) Init(backupPath string) error {
	cmd := fmt.Sprintf(_initFmt, backupPath)

	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("Init output[msg=%s,err=%v]", output, err))

	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("init backup path failure,err=%s,wrap=%w", err, cons.BackupPathAlreadyExist)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) deinit(backupPath string) error {
	if !strings.HasPrefix(backupPath, "/home/omm/") {
		return cons.NoPermission
	}

	cmd := fmt.Sprintf(_rmDirFmt, backupPath)
	if _, err := cmds.Exec(og.shell, cmd); err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) AddInstance(backupPath, instance string) error {
	cmd := fmt.Sprintf(_addInstanceFmt, backupPath, instance, og.pgData)

	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("AddInstance[output=%s,err=%v]", output, err))

	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("add instance failure[output=%s],err=%s,wrap=%w", output, err, cons.InstanceAlreadyExist)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) DelInstance(backupPath, instancee string) error {
	cmd := fmt.Sprintf(_delInstanceFmt, backupPath, instancee)
	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("DelInstance[output=%s,err=%v]", output, err))

	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("delete instance failure[output=%s],err=%s,wrap=%w", output, err, cons.InstanceNotExist)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) Start() error {
	cmd := fmt.Sprintf(_startOpenGaussFmt, og.pgData)
	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("Start openGauss[output=%s]", output))

	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("start openGauss failure[output=%s],err=%s,wrap=%w", output, err, cons.StartOpenGaussFailed)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s,output=%s] return err=%w", og.shell, cmd, output, err)
	}

	return nil
}

func (og *openGauss) Stop() error {
	cmd := fmt.Sprintf(_stopOpenGaussFmt, og.pgData)
	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("Stop openGauss[output=%s]", output))

	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("stop openGauss failure[output=%s],err=%s,wrap=%w", output, err, cons.StopOpenGaussFailed)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s,output=%s] return err=%w", og.shell, cmd, output, err)
	}

	return nil
}

/*
Status return openGauss server status:

		`Running`  return "Runnging",nil
	    `Stopped`   return "Stopped",nil

The others are abnormal states,return "" and error.
*/
func (og *openGauss) Status() (string, error) {
	cmd := fmt.Sprintf(_statusGaussFmt, og.pgData)
	output, err := cmds.Exec(og.shell, cmd)
	og.log.Debug(fmt.Sprintf("Status openGauss[output=%s]", output))

	if errors.Is(err, cons.CmdOperateFailed) {
		if strings.Contains(err.Error(), "no server running") {
			return "Stopped", nil
		}
		return "", fmt.Errorf("get openGauss status failure[output=%s],err=[%s],wrap=%w", output, err, cons.StopOpenGaussFailed)
	}
	if err != nil {
		return "", fmt.Errorf("cmds.Exec[shell=%s,cmd=%s,output=%s] return err=%w", og.shell, cmd, output, err)
	}

	if strings.Contains(output, "server is running") {
		return "Running", nil
	}

	return "", cons.UnknownOgStatus
}

// Restore TODO:Dependent environments require integration testing
func (og *openGauss) Restore(backupPath, instance, backupID string) error {
	cmd := fmt.Sprintf(_restoreFmt, backupPath, instance, backupID, og.pgData)
	outputs, err := cmds.AsyncExec(og.shell, cmd)
	for output := range outputs {
		og.log.
			Fields(map[logging.FieldKey]string{
				"backup_path": backupPath,
				"instance":    instance,
				"backup_id":   backupID,
			}).
			Debug(fmt.Sprintf("Restore openGauss[lineNo=%d,msg=%s]", output.LineNo, output.Message))

		if errors.Is(err, cons.CmdOperateFailed) {
			return fmt.Errorf("cmds.AsyncExec[output=%s] return err=%s,wrap=%w", output.Message, output.Error, cons.RestoreFailed)
		}
		if output.Error != nil {
			return fmt.Errorf("cmds.AsyncExec outputs:Error[%s] is not nil,wrap=%w", output.Error, cons.RestoreFailed)
		}
	}
	return nil
}

func (og *openGauss) ShowBackupList(backupPath, instanceName string) ([]model.Backup, error) {
	cmd := fmt.Sprintf(_showListFmt, instanceName, backupPath)
	output, err := cmds.Exec(og.shell, cmd)
	if err != nil {
		return nil, fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}

	var list []model.BackupList
	if err = json.Unmarshal([]byte(output), &list); err != nil {
		return nil, fmt.Errorf("json.Unmarshal[output=%s] return err=%s,wrap=%w", output, err, cons.Internal)
	}

	for _, ins := range list {
		if ins.Instance == instanceName {
			if len(ins.List) == 0 {
				return nil, fmt.Errorf("instance[name=%s],backupList[v=%+v],err=%w", ins.Instance, list, cons.DataNotFound)
			}

			return ins.List, nil
		}
	}

	return nil, fmt.Errorf("backupList[v=%+v],err=%w", list, cons.DataNotFound)
}

func (og *openGauss) ignore(outputs chan *cmds.Output) {
	defer func() {
		_ = recover()
	}()

	for range outputs {
		//ignore all
	}
	//outputs closed
}

func (og *openGauss) getBackupID(msg string) (string, error) {
	re := regexp2.MustCompile("(?<=backup ID:\\s+)\\w+(?=,)", 0)
	match, err := re.FindStringMatch(msg)
	if err != nil {
		return "", fmt.Errorf("unmatch any backup id[msg=%s],err=%s", msg, err)
	}
	if match.Length == 0 {
		return "", fmt.Errorf("unmatch any backup id,match.lenght is 0,err=%w", cons.UnmatchBackupID)
	}
	return match.String(), err
}

func (og *openGauss) Auth(user, password, dbName string, dbPort uint16) error {
	if strings.Trim(user, " ") == "" ||
		strings.Trim(password, " ") == "" ||
		strings.Trim(dbName, " ") == "" ||
		dbPort == 0 {
		return fmt.Errorf("invalid inputs[user=%s,password=%s,dbName=%s,dbPort=%d]", user, password, dbName, dbPort)
	}

	_og, err := gsutil.Open(user, password, dbName, dbPort)
	if err != nil {
		return fmt.Errorf("gsutil.Open failure,err=%w", err)
	}

	if err := _og.Ping(); err != nil {
		return fmt.Errorf("ping openGauss fail[user=%s,pw length=%d,dbName=%s],err=%w", user, len(password), dbName, err)
	}
	return nil
}

func (og *openGauss) MvPgDataToTemp() error {
	cmd := fmt.Sprintf(_mvFmt, og.pgData, og.pgDataTemp)
	_, err := cmds.Exec(og.shell, cmd)
	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("mv pgdata to temp dir failure,err=%s,wrap=%w", err, cons.MvPgDataToTempFailed)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}

	return nil
}

func (og *openGauss) MvTempToPgData() error {
	cmd := fmt.Sprintf(_mvFmt, og.pgDataTemp, og.pgData)
	_, err := cmds.Exec(og.shell, cmd)
	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("mv temp to pgdata dir failure,err=%s,wrap=%w", err, cons.MvTempToPgDataFailed)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}

func (og *openGauss) CleanPgDataTemp() error {
	cmd := fmt.Sprintf(_rmDirFmt, og.pgDataTemp)
	_, err := cmds.Exec(og.shell, cmd)
	if errors.Is(err, cons.CmdOperateFailed) {
		return fmt.Errorf("clean pgdata temp dir failure,err=%s,wrap=%w", err, cons.CleanPgDataTempFailed)
	}
	if err != nil {
		return fmt.Errorf("cmds.Exec[shell=%s,cmd=%s] return err=%w", og.shell, cmd, err)
	}
	return nil
}
