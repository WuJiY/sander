// Copyright 2017 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"sander/db"
	"sander/logger"
	"sander/model"

	"github.com/polaris1119/set"
	"golang.org/x/net/context"
)

type AdLogic struct{}

var DefaultAd = AdLogic{}

func (AdLogic) FindAll(ctx context.Context, path string) map[string]*model.Advertisement {
	pageAds := make([]*model.PageAd, 0)
	err := db.MasterDB.Where("(path=? OR path=?) AND is_online=1", path, "*").Find(&pageAds)
	if err != nil {
		logger.Error("AdLogic FindAll PageAd error:%+v", err)
		return nil
	}

	adIdSet := set.New(set.NonThreadSafe)
	for _, pageAd := range pageAds {
		adIdSet.Add(pageAd.AdId)
	}

	if adIdSet.IsEmpty() {
		return nil
	}

	adMap := make(map[int]*model.Advertisement)
	err = db.MasterDB.In("id", set.IntSlice(adIdSet)).Find(&adMap)
	if err != nil {
		logger.Error("AdLogic FindAll Advertisement error:%+v", err)
		return nil
	}

	posAdsMap := make(map[string]*model.Advertisement, len(pageAds))
	for _, pageAd := range pageAds {
		if adMap[pageAd.AdId].IsOnline {
			posAdsMap[pageAd.Position] = adMap[pageAd.AdId]
		}
	}

	return posAdsMap
}
