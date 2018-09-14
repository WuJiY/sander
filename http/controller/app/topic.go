// Copyright 2017 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: polaris	polaris@studygolang.com

package app

import (
	"html/template"
	"net/http"

	xhttp "sander/http"
	"sander/http/middleware"
	"sander/logic"
	"sander/model"

	"github.com/labstack/echo"
	"github.com/polaris1119/goutils"
)

// TopicController .
type TopicController struct{}

// RegisterRoute 注册路由
func (t TopicController) RegisterRoute(g *echo.Group) {
	g.GET("/topics", t.TopicList)
	g.GET("/topics/no_reply", t.TopicsNoReply)
	g.GET("/topics/last", t.TopicsLast)
	g.GET("/topic/detail", t.Detail)
	g.GET("/topics/node/:nid", t.NodeTopics)

	g.Match([]string{"GET", "POST"}, "/topics/new", t.Create, middleware.NeedLogin(), middleware.Sensivite(), middleware.PublishNotice())
	g.Match([]string{"GET", "POST"}, "/topics/modify", t.Modify, middleware.NeedLogin(), middleware.Sensivite())
}

// TopicList .
func (t TopicController) TopicList(ctx echo.Context) error {
	tab := ctx.QueryParam("tab")
	if tab != "" && tab != "all" {
		nid := logic.GetNidByEname(tab)
		if nid > 0 {
			return t.topicList(ctx, tab, "topics.mtime DESC", "nid=? AND top!=1", nid)
		}
	}

	return t.topicList(ctx, "all", "topics.mtime DESC", "top!=1")
}

// Topics .
func (t TopicController) Topics(ctx echo.Context) error {
	return t.topicList(ctx, "", "topics.mtime DESC", "")
}

// TopicsNoReply .
func (t TopicController) TopicsNoReply(ctx echo.Context) error {
	return t.topicList(ctx, "no_reply", "topics.mtime DESC", "lastreplyuid=?", 0)
}

// TopicsLast .
func (t TopicController) TopicsLast(ctx echo.Context) error {
	return t.topicList(ctx, "last", "ctime DESC", "")
}

func (TopicController) topicList(ctx echo.Context, tab, orderBy, querystring string, args ...interface{}) error {
	curPage := goutils.MustInt(ctx.QueryParam("p"), 1)
	paginator := logic.NewPaginatorWithPerPage(curPage, perPage)

	// 置顶的topic
	topTopics := logic.DefaultTopic.FindAll(ctx, paginator, "ctime DESC", "top=1")

	topics := logic.DefaultTopic.FindAll(ctx, paginator, orderBy, querystring, args...)
	total := logic.DefaultTopic.Count(ctx, querystring, args...)
	hasMore := paginator.SetTotal(total).HasMorePage()

	hotNodes := logic.DefaultTopic.FindHotNodes(ctx)

	data := map[string]interface{}{
		"topics":   append(topTopics, topics...),
		"tab":      tab,
		"tab_list": hotNodes,
		"has_more": hasMore,
	}

	return success(ctx, data)
}

// NodeTopics 某节点下的主题列表
func (TopicController) NodeTopics(ctx echo.Context) error {
	curPage := goutils.MustInt(ctx.QueryParam("p"), 1)
	paginator := logic.NewPaginator(curPage)

	querystring, nid := "nid=?", goutils.MustInt(ctx.Param("nid"))
	topics := logic.DefaultTopic.FindAll(ctx, paginator, "topics.mtime DESC", querystring, nid)
	total := logic.DefaultTopic.Count(ctx, querystring, nid)
	page := paginator.SetTotal(total).GetPageHtml(ctx.Request().URL().Path())

	// 当前节点信息
	node := logic.GetNode(nid)

	return success(ctx, map[string]interface{}{"activeTopics": "active", "topics": topics, "page": template.HTML(page), "total": total, "node": node})
}

// Detail 社区主题详细页
func (TopicController) Detail(ctx echo.Context) error {
	tid := goutils.MustInt(ctx.QueryParam("tid"))
	if tid == 0 {
		return fail(ctx, "tid 非法")
	}

	topic, replies, err := logic.DefaultTopic.FindByTid(ctx, tid)
	if err != nil {
		return fail(ctx, "服务器异常")
	}

	logic.Views.Incr(xhttp.Request(ctx), model.TypeTopic, tid)

	data := map[string]interface{}{
		"topic":   topic,
		"replies": replies,
	}

	return success(ctx, data)
}

// Create 新建主题
func (TopicController) Create(ctx echo.Context) error {
	nodes := logic.GenNodes()

	title := ctx.FormValue("title")
	// 请求新建主题页面
	if title == "" || ctx.Request().Method() != "POST" {
		return success(ctx, map[string]interface{}{"nodes": nodes, "activeTopics": "active"})
	}

	me := ctx.Get("user").(*model.Me)
	tid, err := logic.DefaultTopic.Publish(ctx, me, ctx.FormParams())
	if err != nil {
		return fail(ctx, "内部服务错误", 1)
	}

	return success(ctx, map[string]interface{}{"tid": tid})
}

// Modify 修改主题
func (TopicController) Modify(ctx echo.Context) error {
	tid := goutils.MustInt(ctx.FormValue("tid"))
	if tid == 0 {
		return ctx.Redirect(http.StatusSeeOther, "/topics")
	}

	nodes := logic.GenNodes()

	if ctx.Request().Method() != "POST" {
		topics := logic.DefaultTopic.FindByTids([]int{tid})
		if len(topics) == 0 {
			return ctx.Redirect(http.StatusSeeOther, "/topics")
		}

		return success(ctx, map[string]interface{}{"nodes": nodes, "topic": topics[0], "activeTopics": "active"})
	}

	me := ctx.Get("user").(*model.Me)
	_, err := logic.DefaultTopic.Publish(ctx, me, ctx.FormParams())
	if err != nil {
		if err == logic.NotModifyAuthorityErr {
			return fail(ctx, "没有权限操作", 1)
		}

		return fail(ctx, "服务错误，请稍后重试！", 2)
	}
	return success(ctx, map[string]interface{}{"tid": tid})
}
