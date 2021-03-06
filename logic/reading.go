// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"sander/db"
	"sander/logger"
	"sander/model"

	"golang.org/x/net/context"
)

type ReadingLogic struct{}

var DefaultReading = ReadingLogic{}

func (ReadingLogic) FindLastList(beginTime string) ([]*model.MorningReading, error) {
	readings := make([]*model.MorningReading, 0)
	err := db.MasterDB.Where("ctime>? AND rtype=0", beginTime).OrderBy("id DESC").Find(&readings)

	return readings, err
}

// 获取晨读列表（分页）
func (ReadingLogic) FindBy(ctx context.Context, limit, rtype int, lastIds ...int) []*model.MorningReading {

	dbSession := db.MasterDB.Where("rtype=?", rtype)
	if len(lastIds) > 0 && lastIds[0] > 0 {
		dbSession.And("id<?", lastIds[0])
	}

	readingList := make([]*model.MorningReading, 0)
	err := dbSession.OrderBy("id DESC").Limit(limit).Find(&readingList)
	if err != nil {
		logger.Error("ResourceLogic FindReadings Error:", err)
		return nil
	}

	return readingList
}

// 【我要晨读】
func (ReadingLogic) IReading(ctx context.Context, id int) string {

	reading := &model.MorningReading{}
	_, err := db.MasterDB.Id(id).Get(reading)
	if err != nil {
		logger.Error("reading logic IReading error:", err)
		return "/readings"
	}

	if reading.Id == 0 {
		return "/readings"
	}

	go db.MasterDB.Id(id).Incr("clicknum", 1).Update(reading)

	if reading.Inner == 0 {
		return "/wr?u=" + reading.Url
	}

	return "/articles/" + strconv.Itoa(reading.Inner)
}

// FindReadingByPage 获取晨读列表（分页）
func (ReadingLogic) FindReadingByPage(ctx context.Context, conds map[string]string, curPage, limit int) ([]*model.MorningReading, int) {
	session := db.MasterDB.NewSession()

	for k, v := range conds {
		session.And(k+"=?", v)
	}

	totalSession := session.Clone()

	offset := (curPage - 1) * limit
	readingList := make([]*model.MorningReading, 0)
	err := session.OrderBy("id DESC").Limit(limit, offset).Find(&readingList)
	if err != nil {
		logger.Error("reading find error:", err)
		return nil, 0
	}

	total, err := totalSession.Count(new(model.MorningReading))
	if err != nil {
		logger.Error("reading find count error:", err)
		return nil, 0
	}

	return readingList, int(total)
}

// SaveReading 保存晨读
func (ReadingLogic) SaveReading(ctx context.Context, form url.Values, username string) (errMsg string, err error) {
	reading := &model.MorningReading{}
	err = schemaDecoder.Decode(reading, form)
	if err != nil {
		logger.Error("reading SaveReading error:%+v", err)
		errMsg = err.Error()
		return
	}

	readings := make([]*model.MorningReading, 0)
	if reading.Inner != 0 {
		reading.Url = ""
		err = db.MasterDB.Where("`inner`=?", reading.Inner).OrderBy("id DESC").Find(&readings)
	} else {
		err = db.MasterDB.Where("url=?", reading.Url).OrderBy("id DESC").Find(&readings)
	}
	if err != nil {
		logger.Error("reading SaveReading MasterDB.Where() error:%+v", err)
		errMsg = err.Error()
		return
	}

	reading.Moreurls = strings.TrimSpace(reading.Moreurls)
	if strings.Contains(reading.Moreurls, "\n") {
		reading.Moreurls = strings.Join(strings.Split(reading.Moreurls, "\n"), ",")
	}

	reading.Username = username

	logger.Debug("typ:%+v,id:%+v", reading.Rtype, reading.Id)
	if reading.Id != 0 {
		_, err = db.MasterDB.Id(reading.Id).Update(reading)
	} else {
		if len(readings) > 0 {
			logger.Error("reading report:%+v", reading)
			errMsg, err = "已经存在了!!", errors.New("已经存在了!!")
			return
		}
		_, err = db.MasterDB.Insert(reading)
	}

	if err != nil {
		errMsg = "内部服务器错误"
		logger.Error("reading save:", errMsg, ":%+v", err)
		return
	}

	return
}

// FindById 获取单条晨读
func (ReadingLogic) FindById(ctx context.Context, id int) *model.MorningReading {
	reading := &model.MorningReading{}
	_, err := db.MasterDB.Id(id).Get(reading)
	if err != nil {
		logger.Error("reading logic FindReadingById Error:%+v", err)
		return nil
	}

	if reading.Id == 0 {
		return nil
	}

	return reading
}
