// Copyright 2017 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"net/url"
	"strings"
	"time"

	"sander/db"
	"sander/logger"
	"sander/model"

	"github.com/sundy-li/html2article"
)

func (self ArticleLogic) ParseArticleByAccuracy(articleUrl string) (*model.Article, error) {
	htmlArticle, err := html2article.FromUrl(articleUrl)
	if err != nil {
		logger.Error("html2article from url:%+v,error:%+v", articleUrl, err)
		return nil, err
	}

	urlTyp, err := url.Parse(articleUrl)
	if err != nil {
		logger.Error("html2article parse url:%+v,error:%+v", articleUrl, err)
		return nil, err
	}

	var (
		title = htmlArticle.Title
		name  = urlTyp.Hostname()
	)
	pos := strings.LastIndex(htmlArticle.Title, "-")
	if pos == -1 {
		pos = strings.LastIndex(htmlArticle.Title, "|")
	}

	if pos != -1 {
		title = strings.TrimSpace(htmlArticle.Title[:pos])
		name = strings.TrimSpace(htmlArticle.Title[pos+1:])
	}

	pubDate := time.Now().Format("2006-01-02 15:04")
	if htmlArticle.Publishtime > 0 {
		pubDate = time.Unix(htmlArticle.Publishtime, 0).UTC().Format("2006-01-02 15:04")
	}
	article := &model.Article{
		Domain:    urlTyp.Hostname(),
		Name:      name,
		Title:     title,
		Author:    name,
		AuthorTxt: name,
		Content:   htmlArticle.Html,
		Txt:       htmlArticle.Content,
		PubDate:   pubDate,
		Url:       articleUrl,
	}

	_, err = db.MasterDB.Insert(article)
	if err != nil {
		logger.Error("insert article error:%+v", err)
		return nil, err
	}

	return article, nil
}
