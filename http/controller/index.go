// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: polaris	polaris@studygolang.com

package controller

import (
	"bytes"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"sander/config"
	xhttp "sander/http"
	"sander/logger"
	"sander/logic"
	"sander/model"

	"github.com/labstack/echo"
	"github.com/polaris1119/goutils"
)

type IndexController struct{}

// 注册路由
func (i IndexController) RegisterRoute(g *echo.Group) {
	g.GET("/", i.Index)
	g.GET("/wr", i.WrapUrl)
	g.GET("/pkgdoc", i.Pkgdoc)
	g.GET("/markdown", i.Markdown)
	g.GET("/link", i.Link)
}

func (IndexController) Index(ctx echo.Context) error {
	if len(logic.WebsiteSetting.IndexNavs) == 0 {
		return render(ctx, "index.html", nil)
	}

	tab := ctx.QueryParam("tab")
	if tab == "" {
		tab = xhttp.GetFromCookie(ctx, "INDEX_TAB")
	}

	if tab == "" {
		tab = logic.WebsiteSetting.IndexNavs[0].Tab
	}
	paginator := logic.NewPaginator(goutils.MustInt(ctx.QueryParam("p"), 1))

	data := logic.DefaultIndex.FindData(ctx, tab, paginator)

	xhttp.SetCookie(ctx, "INDEX_TAB", data["tab"].(string))

	data["all_nodes"] = logic.GenNodes()

	if tab == "all" {
		pageHtml := paginator.SetTotal(logic.DefaultFeed.GetTotalCount(ctx)).GetPageHtml(ctx.Request().URL().Path())

		data["page"] = template.HTML(pageHtml)

		data["total"] = paginator.GetTotal()

	}

	return render(ctx, "index.html", data)
}

// Index 首页
func (IndexController) OldIndex(ctx echo.Context) error {
	num := 10
	paginator := logic.NewPaginatorWithPerPage(1, num)
	topicsList := make([]map[string]interface{}, num)

	// 置顶的topic
	topTopics := logic.DefaultTopic.FindAll(ctx, paginator, "ctime DESC", "top=1")
	if len(topTopics) < num {
		// 获取最新帖子
		paginator.SetPerPage(num - len(topTopics))
		newTopics := logic.DefaultTopic.FindAll(ctx, paginator, "ctime DESC", "top=0")

		topicsList = append(topTopics, newTopics...)
	}

	// 获得最新博文
	recentArticles := logic.DefaultArticle.FindBy(ctx, 10)
	// 获取当前用户喜欢对象信息
	var likeFlags map[int]int

	if len(recentArticles) > 0 {
		curUser, ok := ctx.Get("user").(*model.Me)
		if ok {
			likeFlags, _ = logic.DefaultLike.FindUserLikeObjects(ctx, curUser.Uid, model.TypeArticle, recentArticles[0].Id, recentArticles[len(recentArticles)-1].Id)
		}
	}

	// 资源
	resources := logic.DefaultResource.FindBy(ctx, 10)

	books := logic.DefaultGoBook.FindBy(ctx, 24)
	if len(books) > 8 {
		bookNum := 8
		bookStart := rand.Intn(len(books) - bookNum)
		books = books[bookStart : bookStart+bookNum]
	}

	// 学习资料
	materials := logic.DefaultLearningMaterial.FindAll(ctx)

	return render(ctx, "index.html",
		map[string]interface{}{
			"topics":    topicsList,
			"articles":  recentArticles,
			"likeflags": likeFlags,
			"resources": resources,
			"books":     books,
			"materials": materials,
		})
}

// WrapUrl 包装链接
func (IndexController) WrapUrl(ctx echo.Context) error {
	tUrl := ctx.QueryParam("u")
	if tUrl == "" {
		return ctx.Redirect(http.StatusSeeOther, "/")
	}

	// 本站
	if strings.Contains(tUrl, logic.WebsiteSetting.Domain) {
		return ctx.Redirect(http.StatusSeeOther, tUrl)
	}

	if strings.Contains(tUrl, "?") {
		tUrl += "&"
	} else {
		tUrl += "?"
	}
	tUrl += "utm_campaign=studygolang.com&utm_medium=studygolang.com&utm_source=studygolang.com"

	if xhttp.CheckIsHttps(ctx) {
		return ctx.Redirect(http.StatusSeeOther, tUrl)
	}

	var (
		pUrl *url.URL
		err  error
	)

	if pUrl, err = url.Parse(tUrl); err != nil {
		return ctx.Redirect(http.StatusSeeOther, tUrl)
	}

	iframeDeny := config.ConfigFile.MustValue("crawl", "iframe_deny")
	// 检测是否禁止了 iframe 加载
	// 看是否在黑名单中
	for _, denyHost := range strings.Split(iframeDeny, ",") {
		if strings.Contains(pUrl.Host, denyHost) {
			return ctx.Redirect(http.StatusSeeOther, tUrl)
		}
	}

	// 检测会比较慢，进行异步检测，记录下来，以后分析再加黑名单
	go func() {
		resp, err := http.Head(tUrl)
		if err != nil {
			logger.Error("[iframe] head url:%+v,error:%+v", tUrl, err)
			return
		}
		defer resp.Body.Close()
		if resp.Header.Get("X-Frame-Options") != "" {
			logger.Error("[iframe] deny:%+v", tUrl)
			return
		}
	}()

	return render(ctx, "wr.html", map[string]interface{}{"url": tUrl})
}

// Pkgdoc PkgdocHandler Go 语言文档中文版
func (IndexController) Pkgdoc(ctx echo.Context) error {
	// return render(ctx, "pkgdoc.html", map[string]interface{}{"activeDoc": "active"})
	tpl, err := template.ParseFiles(config.TemplateDir + "pkgdoc.html")
	if err != nil {
		logger.Error("parse file error:%+v", err)
		return err
	}

	buf := new(bytes.Buffer)
	err = tpl.Execute(buf, nil)
	if err != nil {
		logger.Error("execute template error:%+v", err)
		return err
	}

	return ctx.HTML(http.StatusOK, buf.String())
}

// Markdown .
func (IndexController) Markdown(ctx echo.Context) error {
	return render(ctx, "markdown.html", nil)
}

// Link 用于重定向外部链接，比如广告链接
func (IndexController) Link(ctx echo.Context) error {
	tURL := ctx.QueryParam("url")
	if strings.Contains(tURL, "?") {
		tURL += "&"
	} else {
		tURL += "?"
	}
	tURL += "utm_campaign=studygolang.com&utm_medium=studygolang.com&utm_source=studygolang.com"
	return ctx.Redirect(http.StatusSeeOther, tURL)
}
