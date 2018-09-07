package logic

import (
	"bytes"
	"io/ioutil"

	"sander/config"
	"sander/db"
	"sander/logger"
	"sander/model"

	"golang.org/x/net/context"
)

type InstallLogic struct{}

var DefaultInstall = InstallLogic{}

func (InstallLogic) CreateTable(ctx context.Context) error {

	dbFile := config.ROOT + "/config/db.sql"
	buf, err := ioutil.ReadFile(dbFile)

	if err != nil {
		logger.Error("create table, read db file error:", err)
		return err
	}

	sqlSlice := bytes.Split(buf, []byte("CREATE TABLE"))
	db.MasterDB.Exec("SET SQL_MODE='ALLOW_INVALID_DATES';")
	for _, oneSql := range sqlSlice {
		strSql := string(bytes.TrimSpace(oneSql))
		if strSql == "" {
			continue
		}

		strSql = "CREATE TABLE " + strSql
		_, err1 := db.MasterDB.Exec(strSql)
		if err1 != nil {
			logger.Error("create table error:", err1)
			err = err1
		}
	}

	return err
}

// InitTable 初始化数据表
func (InstallLogic) InitTable(ctx context.Context) error {

	total, err := db.MasterDB.Count(new(model.Role))
	if err != nil {
		return err
	}

	if total > 0 {
		return nil
	}

	dbFile := config.ROOT + "/config/init.sql"
	buf, err := ioutil.ReadFile(dbFile)
	if err != nil {
		logger.Error("init table, read init file error:", err)
		return err
	}

	sqlSlice := bytes.Split(buf, []byte("INSERT INTO"))
	for _, oneSql := range sqlSlice {
		strSql := string(bytes.TrimSpace(oneSql))
		if strSql == "" {
			continue
		}

		strSql = "INSERT INTO " + strSql
		_, err1 := db.MasterDB.Exec(strSql)
		if err1 != nil {
			logger.Error("create table error:", err1)
			err = err1
		}
	}

	return err
}

func (InstallLogic) IsTableExist(ctx context.Context) bool {
	exists, err := db.MasterDB.IsTableExist(new(model.User))
	if err != nil || !exists {
		return false
	}

	return true
}

// HadRootUser 是否已经创建了超级用户
func (InstallLogic) HadRootUser(ctx context.Context) bool {
	user := &model.User{}
	_, err := db.MasterDB.Where("is_root=?", 1).Get(user)
	if err != nil {
		// 发生错误，认为已经创建了
		return true
	}

	return user.Uid != 0
}
