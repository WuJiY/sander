// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: polaris	polaris@studygolang.com

package controller

import (
	"fmt"
	"net/http"
	"time"

	xhttp "sander/http"
	"sander/logic"
	"sander/model"

	"github.com/gorilla/feeds"
	"github.com/labstack/echo"
)

type FeedController struct{}

// 注册路由
func (f FeedController) RegisterRoute(g *echo.Group) {
	g.Get("/feed.html", f.Atom)
	g.Get("/feed.xml", f.List)
}

func (f FeedController) Atom(ctx echo.Context) error {
	return xhttp.Render(ctx, "atom.html", map[string]interface{}{})
}

func (f FeedController) List(ctx echo.Context) error {
	link := logic.WebsiteSetting.Domain
	if logic.WebsiteSetting.OnlyHttps {
		link = "https://" + link + "/"
	} else {
		link = "http://" + link + "/"
	}

	now := time.Now()

	feed := &feeds.Feed{
		Title:       logic.WebsiteSetting.Name,
		Link:        &feeds.Link{Href: link},
		Description: logic.WebsiteSetting.Slogan,
		Author:      &feeds.Author{Name: "polaris", Email: "polaris@studygolang.com"},
		Created:     now,
		Updated:     now,
	}

	respBody, err := logic.DefaultSearcher.FindAtomFeeds(50)
	if err != nil {
		return err
	}

	feed.Items = make([]*feeds.Item, len(respBody.Docs))

	for i, doc := range respBody.Docs {
		url := ""

		switch doc.Objtype {
		case model.TypeTopic:
			url = fmt.Sprintf("%stopics/%d", link, doc.Objid)
		case model.TypeArticle:
			url = fmt.Sprintf("%sarticles/%d", link, doc.Objid)
		case model.TypeResource:
			url = fmt.Sprintf("%sresources/%d", link, doc.Objid)
		case model.TypeProject:
			url = fmt.Sprintf("%sp/%d", link, doc.Objid)
		case model.TypeWiki:
			url = fmt.Sprintf("%swiki/%d", link, doc.Objid)
		case model.TypeBook:
			url = fmt.Sprintf("%sbook/%d", link, doc.Objid)
		}
		feed.Items[i] = &feeds.Item{
			Title:       doc.Title,
			Link:        &feeds.Link{Href: url},
			Author:      &feeds.Author{Name: doc.Author},
			Description: doc.Content,
			Created:     time.Time(doc.CreatedAt),
			Updated:     time.Time(doc.CreatedAt),
		}
	}

	atom, err := feed.ToAtom()
	if err != nil {
		return err
	}

	return f.responseXML(ctx, atom)
}

func (FeedController) responseXML(ctx echo.Context, data string) (err error) {
	response := ctx.Response()
	response.Header().Set(echo.HeaderContentType, echo.MIMEApplicationXMLCharsetUTF8)
	response.WriteHeader(http.StatusOK)
	_, err = response.Write([]byte(data))
	return
}
