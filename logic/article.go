// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sander/config"
	"sander/db"
	"sander/global"
	"sander/logger"
	"sander/model"

	"github.com/PuerkitoBio/goquery"
	"github.com/jaytaylor/html2text"
	"github.com/polaris1119/goutils"
	"github.com/polaris1119/set"
	"github.com/polaris1119/slices"
	"github.com/polaris1119/times"
	"github.com/tidwall/gjson"
	"golang.org/x/net/context"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type ArticleLogic struct{}

var DefaultArticle = ArticleLogic{}

var domainPatch = map[string]string{
	"iteye.com":      "iteye.com",
	"blog.51cto.com": "blog.51cto.com",
}

var articleRe = regexp.MustCompile("[\r　\n  \t\v]+")
var articleSpaceRe = regexp.MustCompile("[ ]+")

// ParseArticle 获取 url 对应的文章并根据规则进行解析
func (self ArticleLogic) ParseArticle(ctx context.Context, articleUrl string, auto bool) (*model.Article, error) {
	articleUrl = strings.TrimSpace(articleUrl)
	if !strings.HasPrefix(articleUrl, "http") {
		articleUrl = "http://" + articleUrl
	}

	articleUrl = self.cleanUrl(articleUrl, auto)

	tmpArticle := &model.Article{}
	_, err := db.MasterDB.Where("url=?", articleUrl).Get(tmpArticle)
	if err != nil || tmpArticle.Id != 0 {
		logger.Info(articleUrl, "has exists:%+v", err)
		return nil, errors.New("has exists!")
	}

	urlPaths := strings.SplitN(articleUrl, "/", 5)
	domain := urlPaths[2]

	for k, v := range domainPatch {
		if strings.Contains(domain, k) && !strings.Contains(domain, "www."+k) {
			domain = v
			break
		}
	}

	rule := &model.CrawlRule{}
	_, err = db.MasterDB.Where("domain=?", domain).Get(rule)
	if err != nil {
		logger.Error("find rule by domain error:%+v", err)
		return nil, err
	}

	if rule.Id == 0 {
		return self.ParseArticleByAccuracy(articleUrl)
	}

	// 知乎特殊处理
	// 已经恢复和其他一样了 2018-08-11
	// if domain == "zhuanlan.zhihu.com" {
	// return self.ParseZhihuArticle(ctx, articleUrl, rule)
	// }

	var doc *goquery.Document
	if doc, err = goquery.NewDocument(articleUrl); err != nil {
		logger.Error("goquery newdocument error:%+v", err)
		return nil, err
	}

	author, authorTxt := "", ""
	if rule.InUrl {
		index, err := strconv.Atoi(rule.Author)
		if err != nil {
			logger.Error("author rule is illegal:%+v,error:%+v", rule.Author, err)
			return nil, err
		}
		author = urlPaths[index]
		authorTxt = author
	} else {
		if strings.HasPrefix(rule.Author, ".") || strings.HasPrefix(rule.Author, "#") {
			authorSelection := doc.Find(rule.Author)
			author, err = authorSelection.Html()
			if err != nil {
				logger.Error("goquery parse author error:%+v", err)
				return nil, err
			}

			author = strings.TrimSpace(author)
			authorTxt = strings.TrimSpace(authorSelection.Text())
		} else {
			// 某些个人博客，页面中没有作者的信息，因此，规则中 author 即为 作者
			author = rule.Author
			authorTxt = rule.Author
		}
	}

	title := ""
	doc.Find(rule.Title).Each(func(i int, selection *goquery.Selection) {
		if title != "" {
			return
		}

		tmpTitle := strings.TrimSpace(selection.Text())
		tmpTitle = strings.TrimSpace(strings.TrimPrefix(tmpTitle, "原"))
		tmpTitle = strings.TrimSpace(strings.TrimPrefix(tmpTitle, "荐"))
		tmpTitle = strings.TrimSpace(strings.TrimPrefix(tmpTitle, "转"))
		tmpTitle = strings.TrimSpace(strings.TrimPrefix(tmpTitle, "顶"))
		if tmpTitle != "" {
			title = tmpTitle
		}
	})

	if title == "" {
		logger.Error("url:%+v,parse title error:%+v", articleUrl, err)
		return nil, err
	}

	replacer := strings.NewReplacer("[置顶]", "", "[原]", "", "[转]", "")
	title = strings.TrimSpace(replacer.Replace(title))

	contentSelection := doc.Find(rule.Content)

	// 对方图片是否禁止访问
	imgDeny := false
	extMap := rule.ParseExt()
	if extMap != nil {
		if deny, ok := extMap["img_deny"]; ok {
			imgDeny = goutils.MustBool(deny)
		}
	}

	// relative url -> abs url
	contentSelection.Find("img").Each(func(i int, s *goquery.Selection) {
		self.transferImage(ctx, s, imgDeny, domain)
	})

	content, err := contentSelection.Html()
	if err != nil {
		logger.Error("goquery parse content error:%+v", err)
		return nil, err
	}
	content = strings.TrimSpace(content)
	txt := strings.TrimSpace(contentSelection.Text())
	txt = articleRe.ReplaceAllLiteralString(txt, " ")
	txt = articleSpaceRe.ReplaceAllLiteralString(txt, " ")

	// 自动抓取，内容长度不能少于 300 字
	if auto && len(txt) < 300 {
		logger.Error("content is short:%+v", articleUrl)
		return nil, errors.New("content is short")
	}

	if auto && strings.Count(content, "<a") > config.ConfigFile.MustInt("crawl", "contain_link", 10) {
		logger.Error("content contains too many link!,articleUrl:%+v", articleUrl)
		return nil, errors.New("content contains too many link")
	}

	pubDate := times.Format("Y-m-d H:i:s")
	if rule.PubDate != "" {
		pubDate = strings.TrimSpace(doc.Find(rule.PubDate).First().Text())
	}

	if pubDate == "" {
		pubDate = times.Format("Y-m-d H:i:s")
	} else {
		// YYYYY-MM-dd HH:mm
		if len(pubDate) == 16 && auto {
			// 三个月之前不入库
			pubTime, err := time.ParseInLocation("2006-01-02 15:04", pubDate, time.Local)
			if err == nil {
				if pubTime.Add(3 * 30 * 86400 * time.Second).Before(time.Now()) {
					return nil, errors.New("article is old!")
				}
			}
		}
	}

	article := &model.Article{
		Domain:    domain,
		Name:      rule.Name,
		Author:    author,
		AuthorTxt: authorTxt,
		Title:     title,
		Content:   content,
		Txt:       txt,
		PubDate:   pubDate,
		Url:       articleUrl,
		Lang:      rule.Lang,
	}

	if extMap != nil {
		err = self.convertByExt(extMap, article)
		if err != nil {
			return nil, err
		}
	}

	_, err = db.MasterDB.Insert(article)
	if err != nil {
		logger.Error("insert article error:%+v", err)
		return nil, err
	}

	return article, nil
}

func (self ArticleLogic) ParseZhihuArticle(ctx context.Context, articleUrl string, rule *model.CrawlRule) (*model.Article, error) {
	var (
		doc *goquery.Document
		err error
	)
	if doc, err = goquery.NewDocument(articleUrl); err != nil {
		logger.Error("goquery newdocument error:%+v", err)
		return nil, err
	}

	var (
		jsonContentKey string
		ok             bool
	)

	extMap := rule.ParseExt()
	if jsonContentKey, ok = extMap["json_content"]; !ok {
		return nil, errors.New("zhihu config error, not json_content key")
	}

	jsonContent := doc.Find(jsonContentKey).Text()
	if jsonContent == "" {
		return nil, errors.New("zhihu json content is empty")
	}

	pos := strings.LastIndex(articleUrl, "/")
	articleId := articleUrl[pos+1:]

	result := gjson.Parse(jsonContent)
	database := result.Get("database")
	post := database.Get("Post").Get(articleId)
	author := database.Get("User").Get(post.Get("author").String()).Get("name").String()
	content := post.Get("content").String()
	txt, _ := html2text.FromString(content)
	pubDate, _ := time.Parse("2006-01-02T15:04:05+08:00", post.Get("publishedTime").String())

	article := &model.Article{
		Domain:    rule.Domain,
		Name:      rule.Name,
		Author:    author,
		AuthorTxt: author,
		Title:     post.Get("title").String(),
		Content:   content,
		Txt:       txt,
		PubDate:   times.Format("Y-m-d H:i:s", pubDate),
		Url:       articleUrl,
		Lang:      rule.Lang,
	}

	_, err = db.MasterDB.Insert(article)
	if err != nil {
		logger.Error("insert article error:%+v", err)
		return nil, err
	}

	return article, nil
}

// Publish 发布文章
func (self ArticleLogic) Publish(ctx context.Context, me *model.Me, form url.Values) (int, error) {
	var uid = me.Uid
	article := &model.Article{
		Domain:    WebsiteSetting.Domain,
		Name:      WebsiteSetting.Name,
		Author:    me.Username,
		AuthorTxt: me.Username,
		Title:     form.Get("title"),
		Cover:     form.Get("cover"),
		Content:   form.Get("content"),
		Txt:       form.Get("txt"),
		Markdown:  goutils.MustBool(form.Get("markdown"), false),
		PubDate:   times.Format("Y-m-d H:i:s"),
		GCTT:      goutils.MustBool(form.Get("gctt"), false),
	}

	if article.Txt == "" {
		article.Txt = article.Content
	}

	requestIdInter := ctx.Value("request_id")
	if requestIdInter != nil {
		if requestId, ok := requestIdInter.(string); ok {
			article.Url = requestId
		}
	}
	if article.Url == "" {
		logger.Error("request_id is empty!")
		// 理论上不会执行
		return 0, errors.New("request_id is empty!")
	}

	// GCTT 译文，如果译者关联了本站账号，author 改为译者
	if article.GCTT {
		translator := form.Get("translator")
		gcttUser := &model.GCTTUser{}
		_, err := db.MasterDB.Where("username=?", translator).Get(gcttUser)
		if err != nil {
			logger.Error("article publish find gctt user error:%+v", err)
		}

		if gcttUser.Uid > 0 {
			user := DefaultUser.findUser(ctx, gcttUser.Uid)
			article.Author = user.Username
			article.AuthorTxt = user.Username

			uid = user.Uid

			// 【编辑】
			article.OpUser = me.Username
		}
	}

	session := db.MasterDB.NewSession()
	defer session.Close()
	session.Begin()

	_, err := session.Insert(article)
	if err != nil {
		session.Rollback()
		logger.Error("insert article error:%+v", err)
		return 0, err
	}

	change := map[string]interface{}{
		"url": article.Id,
	}
	session.Table(new(model.Article)).Id(article.Id).Update(change)

	if article.GCTT {
		articleGCTT := &model.ArticleGCTT{
			ArticleID:  article.Id,
			Author:     form.Get("author"),
			AuthorURL:  form.Get("author_url"),
			Translator: form.Get("translator"),
			Checker:    form.Get("checker"),
			URL:        form.Get("url"),
		}

		_, err = session.Insert(articleGCTT)
		if err != nil {
			session.Rollback()
			logger.Error("insert article_gctt error:%+v", err)
			return 0, err
		}
	}

	session.Commit()

	go publishObservable.NotifyObservers(uid, model.TypeArticle, article.Id)

	return article.Id, nil
}

func (self ArticleLogic) PublishFromAdmin(ctx context.Context, me *model.Me, form url.Values) error {
	articleUrl := form.Get("url")
	netUrl, err := url.Parse(articleUrl)
	if err != nil {
		logger.Error("url is illegal:%+v", netUrl)
		return err
	}

	article := &model.Article{
		Domain:    netUrl.Host,
		Name:      form.Get("name"),
		Url:       articleUrl,
		Author:    form.Get("author"),
		AuthorTxt: form.Get("author"),
		Title:     form.Get("title"),
		Content:   form.Get("content"),
		Txt:       form.Get("txt"),
		PubDate:   form.Get("pub_date"),
		Lang:      goutils.MustInt(form.Get("lang")),
		Cover:     form.Get("cover"),
	}

	_, err = db.MasterDB.Insert(article)
	if err != nil {
		logger.Error("insert article error:%+v", err)
		return err
	}

	return nil
}

func (ArticleLogic) cleanUrl(articleUrl string, auto bool) string {
	pos := strings.LastIndex(articleUrl, "#")
	if pos > 0 {
		articleUrl = articleUrl[:pos]
	}
	// 过滤多余的参数，避免加一个参数就是一个新文章，但实际上是同一篇
	if auto {
		pos = strings.Index(articleUrl, "?")
		if pos > 0 {
			articleUrl = articleUrl[:pos]
		}
	}

	return articleUrl
}

func (ArticleLogic) convertByExt(extMap map[string]string, article *model.Article) error {
	var err error
	if css, ok := extMap["css"]; ok {
		article.Css = css
	}

	if charset, ok := extMap["charset"]; ok {
		if charset == "gbk" {
			article.Title, err = simplifiedchinese.GBK.NewDecoder().String(article.Title)
			if err != nil {
				logger.Error("convert title gbk to utf8 error:%+v", err)
				return err
			}
			article.Content, err = simplifiedchinese.GBK.NewDecoder().String(article.Content)
			if err != nil {
				logger.Error("convert content gbk to utf8 error:%+v", err)
				return err
			}
			article.Txt, err = simplifiedchinese.GBK.NewDecoder().String(article.Txt)
			if err != nil {
				logger.Error("convert txt gbk to utf8 error:%+v", err)
				return err
			}
			article.AuthorTxt, err = simplifiedchinese.GBK.NewDecoder().String(article.AuthorTxt)
			if err != nil {
				logger.Error("convert txt gbk to utf8 error:%+v", err)
				return err
			}
			article.Author = article.AuthorTxt
		}
	}

	return nil
}

func (ArticleLogic) FindLastList(beginTime string, limit int) ([]*model.Article, error) {
	articles := make([]*model.Article, 0)
	err := db.MasterDB.Where("ctime>? AND status!=?", beginTime, model.ArticleStatusOffline).
		OrderBy("cmtnum DESC, likenum DESC, viewnum DESC").Limit(limit).Find(&articles)

	return articles, err
}

// Total 博文总数
func (ArticleLogic) Total() int64 {
	total, err := db.MasterDB.Count(new(model.Article))
	if err != nil {
		logger.Error("ArticleLogic Total error:%+v", err)
	}
	return total
}

// FindBy 获取抓取的文章列表（分页）
func (self ArticleLogic) FindBy(ctx context.Context, limit int, lastIds ...int) []*model.Article {
	dbSession := db.MasterDB.Where("status IN(?,?)", model.ArticleStatusNew, model.ArticleStatusOnline)

	if len(lastIds) > 0 && lastIds[0] > 0 {
		dbSession.And("id<?", lastIds[0])
	}

	articles := make([]*model.Article, 0)
	err := dbSession.OrderBy("id DESC").Limit(limit).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic FindBy Error:%+v", err)
		return nil
	}

	topArticles := make([]*model.Article, 0)
	err = db.MasterDB.Where("top=?", 1).OrderBy("id DESC").Find(&topArticles)
	if err != nil {
		logger.Error("ArticleLogic Find Top Articles Error:%+v", err)
		return nil
	}
	if len(topArticles) > 0 {
		articles = append(topArticles, articles...)
	}

	self.fillUser(articles)

	return articles
}

func (self ArticleLogic) FindTaGCTTArticles(ctx context.Context, translator string) []*model.Article {
	articleGCTTs := make([]*model.ArticleGCTT, 0)
	err := db.MasterDB.Where("translator=?", translator).OrderBy("article_id DESC").Find(&articleGCTTs)
	if err != nil {
		logger.Error("ArticleLogic FindTaGCTTArticles gctt error:%+v", err)
		return nil
	}
	articleIds := make([]int, len(articleGCTTs))
	for i, articleGCTT := range articleGCTTs {
		articleIds[i] = articleGCTT.ArticleID
	}

	articleMap := make(map[int]*model.Article, 0)
	err = db.MasterDB.In("id", articleIds).Find(&articleMap)
	if err != nil {
		logger.Error("ArticleLogic FindTaGCTTArticles article error:%+v", err)
		return nil
	}

	articles := make([]*model.Article, 0, len(articleMap))
	for _, articleGCTT := range articleGCTTs {
		articleId := articleGCTT.ArticleID

		if article, ok := articleMap[articleId]; ok {
			articles = append(articles, article)
		}
	}

	return articles
}

func (self ArticleLogic) FindByUser(ctx context.Context, username string, limit int) []*model.Article {
	articles := make([]*model.Article, 0)
	err := db.MasterDB.Where("author_txt=?", username).OrderBy("id DESC").Limit(limit).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic FindByUser Error:%+v", err)
		return nil
	}

	return articles
}

func (self ArticleLogic) SearchMyArticles(ctx context.Context, me *model.Me, sid int, kw string) []map[string]interface{} {
	articles := make([]*model.Article, 0)
	session := db.MasterDB.Where("author_txt=?", me.Username).OrderBy("id DESC").Limit(8)
	if kw != "" {
		session.Where("title LIKE ?", "%"+kw+"%")
	}
	err := session.Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic SearchMyArticles Error:%+v", err)
		return nil
	}

	subjectArticles := make([]*model.SubjectArticle, 0)
	articleIds := slices.StructsIntSlice(articles, "Id")
	err = db.MasterDB.Where("sid=?", sid).In("article_id", articleIds).Find(&subjectArticles)
	if err != nil {
		logger.Error("ArticleLogic SearchMyArticles find subject article Error:%+v", err)
		return nil
	}

	subjectArticleMap := make(map[int]struct{})
	for _, subjectArticle := range subjectArticles {
		subjectArticleMap[subjectArticle.ArticleId] = struct{}{}
	}

	articleMapSlice := make([]map[string]interface{}, len(articles))
	for i, article := range articles {
		articleMap := map[string]interface{}{
			"id":    article.Id,
			"title": article.Title,
		}
		if _, ok := subjectArticleMap[article.Id]; ok {
			articleMap["had_add"] = 1
		} else {
			articleMap["had_add"] = 0
		}

		articleMapSlice[i] = articleMap
	}

	return articleMapSlice
}

// FindAll 支持多页翻看
func (self ArticleLogic) FindAll(ctx context.Context, paginator *Paginator, orderBy string, querystring string, args ...interface{}) []*model.Article {
	articles := make([]*model.Article, 0)
	session := db.MasterDB.OrderBy(orderBy)
	if querystring != "" {
		session.Where(querystring, args...)
	}
	err := session.Limit(paginator.PerPage(), paginator.Offset()).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic FindAll error:%+v", err)
		return nil
	}

	self.fillUser(articles)

	return articles
}

func (ArticleLogic) Count(ctx context.Context, querystring string, args ...interface{}) int64 {
	var (
		total int64
		err   error
	)
	if querystring == "" {
		total, err = db.MasterDB.Count(new(model.Article))
	} else {
		total, err = db.MasterDB.Where(querystring, args...).Count(new(model.Article))
	}

	if err != nil {
		logger.Error("ArticleLogic Count error:%+v", err)
	}

	return total
}

// 获取抓取的文章列表（分页）：后台用
func (ArticleLogic) FindArticleByPage(ctx context.Context, conds map[string]string, curPage, limit int) ([]*model.Article, int) {
	session := db.MasterDB.NewSession()

	for k, v := range conds {
		session.And(k+"=?", v)
	}

	totalSession := session.Clone()

	offset := (curPage - 1) * limit
	articleList := make([]*model.Article, 0)
	err := session.OrderBy("id DESC").Limit(limit, offset).Find(&articleList)
	if err != nil {
		logger.Error("find error:%+v", err)
		return nil, 0
	}

	total, err := totalSession.Count(new(model.Article))
	if err != nil {
		logger.Error("find count error:%+v", err)
		return nil, 0
	}

	return articleList, int(total)
}

// FindByIds 获取多个文章详细信息
func (self ArticleLogic) FindByIds(ids []int) []*model.Article {
	if len(ids) == 0 {
		return nil
	}
	articles := make([]*model.Article, 0)
	err := db.MasterDB.In("id", ids).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic FindByIds error:%+v", err)
		return nil
	}

	self.fillUser(articles)

	return articles
}

// MoveToTopic 将该文章移到主题中
// 有些用户总是将问题放在文章中发布
func (self ArticleLogic) MoveToTopic(ctx context.Context, id interface{}, me *model.Me) error {
	article := &model.Article{}
	_, err := db.MasterDB.Id(id).Get(article)
	if err != nil {
		logger.Error("ArticleLogic MoveToTopic find article error:%+v", err)
		return err
	}

	if !article.IsSelf {
		return errors.New("不是本站发布的文章，不能移动！")
	}

	user := DefaultUser.FindOne(ctx, "username", article.AuthorTxt)

	session := db.MasterDB.NewSession()
	defer session.Close()
	session.Begin()

	// TODO: 先不考虑内容非 markdown 格式的情况
	topic := &model.Topic{
		Title:         article.Title,
		Content:       article.Content,
		Nid:           6, // 默认放入问答节点
		Uid:           user.Uid,
		Lastreplyuid:  article.Lastreplyuid,
		Lastreplytime: article.Lastreplytime,
		EditorUid:     me.Uid,
		Tags:          article.Tags,
		Ctime:         article.Ctime,
	}
	_, err = session.Insert(topic)
	if err != nil {
		session.Rollback()
		logger.Error("ArticleLogic MoveToTopic insert Topic error:%+v", err)
		return err
	}

	topicEx := &model.TopicEx{
		Tid:   topic.Tid,
		View:  article.Viewnum,
		Reply: article.Cmtnum,
		Like:  article.Likenum,
	}

	_, err = session.Insert(topicEx)
	if err != nil {
		session.Rollback()
		logger.Error("ArticleLogic MoveToTopic Insert TopicEx error:%+v", err)
		return err
	}

	// 修改动态信息
	_, err = session.Table("feed").
		Where("objid=? AND objtype=?", article.Id, model.TypeArticle).
		Update(map[string]interface{}{
			"objid":   topic.Tid,
			"objtype": model.TypeTopic,
			"nid":     topic.Nid,
		})
	if err != nil {
		session.Rollback()
		logger.Error("ArticleLogic MoveToTopic Update Feed error:%+v", err)
		return err
	}

	// 如果有评论，更新评论属主
	if article.Cmtnum > 0 {
		_, err = session.Table("comments").
			Where("objid=? AND objtype=?", article.Id, model.TypeArticle).
			Update(map[string]interface{}{
				"objid":   topic.Tid,
				"objtype": model.TypeTopic,
			})
		if err != nil {
			session.Rollback()
			logger.Error("ArticleLogic MoveToTopic Update Comment error:%+v", err)
			return err
		}

		// 处理系统消息
		systemMsgs := make([]*model.SystemMessage, 0)
		err = session.Where("`to`=?", user.Uid).Limit(article.Cmtnum).Find(&systemMsgs)
		if err != nil {
			session.Rollback()
			logger.Error("ArticleLogic MoveToTopic find system message error:%+v", err)
			return err
		}

		for _, msg := range systemMsgs {
			extMap := msg.GetExt()

			if val, ok := extMap["objid"]; ok {
				objid := int(val.(float64))
				if objid != article.Id {
					continue
				}

				extMap["objid"] = topic.Tid
				extMap["objtype"] = model.TypeTopic

				msg.SetExt(extMap)

				_, err = session.Id(msg.Id).Update(msg)
				if err != nil {
					session.Rollback()
					logger.Error("ArticleLogic MoveToTopic update system message error:%+v", err)
					return err
				}
			}
		}
	}

	// 减积分处罚作者
	award := -20
	desc := fmt.Sprintf(`你的《%s》并非文章，应该发布到主题中，已被管理员移到主题里 <a href="/topics/%d">%s</a>`, article.Title, topic.Tid, topic.Title)
	DefaultUserRich.IncrUserRich(user, model.MissionTypePunish, award, desc)

	// 将文章删除
	_, err = session.Id(article.Id).Delete(article)

	session.Commit()

	return nil
}

func (self ArticleLogic) transferImage(ctx context.Context, s *goquery.Selection, imgDeny bool, domain string) {
	if v, ok := s.Attr("data-original-src"); ok {
		self.setImgSrc(ctx, v, imgDeny, s)
	} else if v, ok := s.Attr("data-src"); ok {
		self.setImgSrc(ctx, v, imgDeny, s)
	} else if v, ok := s.Attr("src"); ok {
		if !strings.HasPrefix(v, "http") {
			v = "http://" + domain + "/" + v
		}

		self.setImgSrc(ctx, v, imgDeny, s)
	}
}

func (self ArticleLogic) setImgSrc(ctx context.Context, v string, imgDeny bool, s *goquery.Selection) {
	if imgDeny {
		path, err := DefaultUploader.TransferUrl(ctx, v)
		if err == nil {
			s.SetAttr("src", global.App.CDNHttp+path)
		} else {
			s.SetAttr("src", v)
		}
	} else {
		s.SetAttr("src", v)
	}
}

func (ArticleLogic) fillUser(articles []*model.Article) {
	usernameSet := set.New(set.NonThreadSafe)
	uidSet := set.New(set.NonThreadSafe)
	for _, article := range articles {
		if article.IsSelf {
			usernameSet.Add(article.Author)
		}

		if article.Lastreplyuid != 0 {
			uidSet.Add(article.Lastreplyuid)
		}
	}
	if !usernameSet.IsEmpty() {
		userMap := DefaultUser.FindUserInfos(nil, set.StringSlice(usernameSet))
		for _, article := range articles {
			if !article.IsSelf {
				continue
			}

			for _, user := range userMap {
				if article.Author == user.Username {
					article.User = user
					break
				}
			}
		}
	}

	if !uidSet.IsEmpty() {
		replyUserMap := DefaultUser.FindUserInfos(nil, set.IntSlice(uidSet))
		for _, article := range articles {
			if article.Lastreplyuid == 0 {
				continue
			}

			article.LastReplyUser = replyUserMap[article.Lastreplyuid]
		}
	}
}

// findByIds 获取多个文章详细信息 包内使用
func (ArticleLogic) findByIds(ids []int) map[int]*model.Article {
	if len(ids) == 0 {
		return nil
	}
	articles := make(map[int]*model.Article)
	err := db.MasterDB.In("id", ids).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic findByIds error:%+v", err)
		return nil
	}
	return articles
}

// FindByIdAndPreNext 获取当前(id)博文以及前后博文
func (ArticleLogic) FindByIdAndPreNext(ctx context.Context, id int) (curArticle *model.Article, prevNext []*model.Article, err error) {
	if id == 0 {
		err = errors.New("id 不能为0")
		return
	}

	articles := make([]*model.Article, 0)

	err = db.MasterDB.Where("id BETWEEN ? AND ? AND status!=?", id-5, id+5, model.ArticleStatusOffline).Find(&articles)
	if err != nil {
		logger.Error("ArticleLogic FindByIdAndPreNext Error:%+v", err)
		return
	}

	if len(articles) == 0 {
		logger.Error("ArticleLogic FindByIdAndPreNext not find articles, id:%+v", id)
		return
	}

	prevNext = make([]*model.Article, 2)
	prevId, nextId := articles[0].Id, articles[len(articles)-1].Id
	for _, article := range articles {
		if article.Id < id && article.Id > prevId {
			prevId = article.Id
			prevNext[0] = article
		} else if article.Id > id && article.Id < nextId {
			nextId = article.Id
			prevNext[1] = article
		} else if article.Id == id {
			curArticle = article
		}
	}

	if curArticle == nil {
		logger.Error("ArticleLogic FindByIdAndPreNext not find current article, id:%+v", id)
		return
	}

	if prevId == id {
		prevNext[0] = nil
	}

	if nextId == id {
		prevNext[1] = nil
	}

	if curArticle.IsSelf {
		curArticle.User = DefaultUser.FindOne(ctx, "username", curArticle.Author)
	}

	return
}

func (ArticleLogic) FindArticleGCTT(ctx context.Context, article *model.Article) *model.ArticleGCTT {
	articleGCTT := &model.ArticleGCTT{}

	if !article.GCTT {
		return articleGCTT
	}

	_, err := db.MasterDB.Where("article_id=?", article.Id).Get(articleGCTT)
	if err != nil {
		logger.Error("ArticleLogic FindArticleGCTT error:", err)
	}

	if articleGCTT.ArticleID > 0 {
		gcttUser := DefaultGCTT.FindOne(ctx, articleGCTT.Translator)
		articleGCTT.Avatar = gcttUser.Avatar
	}

	return articleGCTT
}

// Modify 修改文章信息
func (ArticleLogic) Modify(ctx context.Context, user *model.Me, form url.Values) (errMsg string, err error) {
	id := form.Get("id")

	article := &model.Article{}
	_, err = db.MasterDB.Id(id).Get(article)
	if err != nil {
		errMsg = "对不起，服务器内部错误，请稍后再试！"
		return
	}

	if !CanEdit(user, article) {
		err = NotModifyAuthorityErr
		return
	}

	form.Set("op_user", user.Username)

	fields := []string{
		"title", "url", "cover", "author", "author_txt",
		"lang", "pub_date", "content",
		"tags", "status", "op_user",
	}
	change := make(map[string]string)

	for _, field := range fields {
		val := form.Get(field)
		if val != "" {
			change[field] = form.Get(field)
		}
	}

	_, err = db.MasterDB.Table(new(model.Article)).Id(id).Update(change)
	if err != nil {
		logger.Error("更新文章 【%s】 信息失败：%s\n", id, err)
		errMsg = "对不起，服务器内部错误，请稍后再试！"
		return
	}

	go modifyObservable.NotifyObservers(user.Uid, model.TypeArticle, goutils.MustInt(id))

	return
}

// FindById 获取单条博文
func (ArticleLogic) FindById(ctx context.Context, id interface{}) (*model.Article, error) {
	article := &model.Article{}
	_, err := db.MasterDB.Id(id).Get(article)
	if err != nil {
		logger.Error("article logic FindById Error:%+v", err)
	}

	return article, err
}

// getOwner 通过objid获得 article 的所有者
func (ArticleLogic) getOwner(id int) int {
	article := &model.Article{}
	_, err := db.MasterDB.Id(id).Get(article)
	if err != nil {
		logger.Error("article logic getOwner Error:%+v", err)
		return 0
	}

	if article.IsSelf {
		user := DefaultUser.FindOne(nil, "username", article.Author)
		return user.Uid
	}
	return 0
}

// 博文评论
type ArticleComment struct{}

// UpdateComment 更新该文章的评论信息
// cid：评论id；objid：被评论对象id；uid：评论者；cmttime：评论时间
func (self ArticleComment) UpdateComment(cid, objid, uid int, cmttime time.Time) {
	// 更新最后回复信息
	_, err := db.MasterDB.Table(new(model.Article)).Id(objid).Incr("cmtnum", 1).Update(map[string]interface{}{
		"lastreplyuid":  uid,
		"lastreplytime": cmttime,
	})
	if err != nil {
		logger.Error("更新回复信息失败：%+v", err)
		return
	}
}

func (self ArticleComment) String() string {
	return "article"
}

// SetObjinfo 实现 CommentObjecter 接口
func (self ArticleComment) SetObjinfo(ids []int, commentMap map[int][]*model.Comment) {
	articles := DefaultArticle.FindByIds(ids)
	if len(articles) == 0 {
		return
	}

	for _, article := range articles {
		objinfo := make(map[string]interface{})
		objinfo["title"] = article.Title
		objinfo["uri"] = model.PathUrlMap[model.TypeArticle]
		objinfo["type_name"] = model.TypeNameMap[model.TypeArticle]

		for _, comment := range commentMap[article.Id] {
			comment.Objinfo = objinfo
		}
	}
}

// 博文喜欢
type ArticleLike struct{}

// 更新该文章的喜欢数
// objid：被喜欢对象id；num: 喜欢数(负数表示取消喜欢)
func (self ArticleLike) UpdateLike(objid, num int) {
	// 更新喜欢数（TODO：暂时每次都更新表）
	_, err := db.MasterDB.Where("id=?", objid).Incr("likenum", num).Update(new(model.Article))
	if err != nil {
		logger.Error("更新文章喜欢数失败：%+v", err)
	}
}

func (self ArticleLike) String() string {
	return "article"
}
