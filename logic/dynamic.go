// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"sander/db"
	"sander/logger"
	"sander/model"

	"golang.org/x/net/context"
)

type DynamicLogic struct{}

var DefaultDynamic = DynamicLogic{}

// FindBy 获取动态列表（分页）
func (DynamicLogic) FindBy(ctx context.Context, lastId int, limit int) []*model.Dynamic {
	dynamicList := make([]*model.Dynamic, 0)
	err := db.MasterDB.Where("id>?", lastId).OrderBy("seq DESC").Limit(limit).Find(&dynamicList)
	if err != nil {
		logger.Error("DynamicLogic FindBy Error:%+v", err)
	}

	return dynamicList
}
