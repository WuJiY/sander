// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author:polaris	polaris@studygolang.com

package logic

import (
	"net/url"
	"strconv"
	"time"

	"sander/db"
	"sander/logger"
	"sander/model"

	"github.com/fatih/structs"
	"github.com/polaris1119/set"
	"golang.org/x/net/context"
)

type ResourceLogic struct{}

var DefaultResource = ResourceLogic{}

// Publish 增加（修改）资源
func (ResourceLogic) Publish(ctx context.Context, me *model.Me, form url.Values) (err error) {

	uid := me.Uid
	resource := &model.Resource{}

	if form.Get("id") != "" {
		id := form.Get("id")
		_, err = db.MasterDB.Id(id).Get(resource)
		if err != nil {
			logger.Error("ResourceLogic Publish find error:%+v", err)
			return
		}

		if !CanEdit(me, resource) {
			err = NotModifyAuthorityErr
			return
		}

		fields := []string{"title", "catid", "form", "url", "content"}
		if form.Get("form") == model.LinkForm {
			form.Set("content", "")
		} else {
			form.Set("url", "")
		}

		for _, field := range fields {
			form.Del(field)
		}

		err = schemaDecoder.Decode(resource, form)
		if err != nil {
			logger.Error("ResourceLogic Publish decode error:", err)
			return
		}
		_, err = db.MasterDB.Id(id).Update(resource)
		if err != nil {
			logger.Error("更新资源 【%s】 信息失败：%s\n", id, err)
			return
		}

		go modifyObservable.NotifyObservers(uid, model.TypeResource, resource.Id)

	} else {

		err = schemaDecoder.Decode(resource, form)
		if err != nil {
			logger.Error("ResourceLogic Publish decode error:", err)
			return
		}

		resource.Uid = uid

		session := db.MasterDB.NewSession()
		defer session.Close()

		err = session.Begin()
		if err != nil {
			session.Rollback()
			logger.Error("Publish Resource begin tx error:", err)
			return
		}

		_, err = session.Insert(resource)
		if err != nil {
			session.Rollback()
			logger.Error("Publish Resource insert resource error:", err)
			return
		}

		resourceEx := &model.ResourceEx{
			Id: resource.Id,
		}
		_, err = session.Insert(resourceEx)
		if err != nil {
			session.Rollback()
			logger.Error("Publish Resource insert resource_ex error:", err)
			return
		}

		err = session.Commit()
		if err != nil {
			logger.Error("Publish Resource commit error:", err)
			return
		}

		// 发布动态
		DefaultFeed.publish(resource, resourceEx)

		// 给 被@用户 发系统消息
		ext := map[string]interface{}{
			"objid":   resource.Id,
			"objtype": model.TypeResource,
			"uid":     uid,
			"msgtype": model.MsgtypePublishAtMe,
		}
		go DefaultMessage.SendSysMsgAtUsernames(ctx, form.Get("usernames"), ext, 0)

		go publishObservable.NotifyObservers(uid, model.TypeResource, resource.Id)
	}

	return
}

// Total 资源总数
func (ResourceLogic) Total() int64 {
	total, err := db.MasterDB.Count(new(model.Resource))
	if err != nil {
		logger.Error("CommentLogic Total error:%+v", err)
	}
	return total
}

// FindBy 获取资源列表（分页）
func (ResourceLogic) FindBy(ctx context.Context, limit int, lastIds ...int) []*model.Resource {

	dbSession := db.MasterDB.OrderBy("id DESC").Limit(limit)
	if len(lastIds) > 0 && lastIds[0] > 0 {
		dbSession.Where("id<?", lastIds[0])
	}

	resourceList := make([]*model.Resource, 0)
	err := dbSession.Find(&resourceList)
	if err != nil {
		logger.Error("ResourceLogic FindBy Error:%+v", err)
		return nil
	}

	return resourceList
}

// FindAll 获得资源列表（完整信息），分页
func (self ResourceLogic) FindAll(ctx context.Context, paginator *Paginator, orderBy, querystring string, args ...interface{}) (resources []map[string]interface{}, total int64) {

	var (
		count         = paginator.PerPage()
		resourceInfos = make([]*model.ResourceInfo, 0)
	)

	session := db.MasterDB.Join("INNER", "resource_ex", "resource.id=resource_ex.id")
	if querystring != "" {
		session.Where(querystring, args...)
	}
	err := session.OrderBy(orderBy).Limit(count, paginator.Offset()).Find(&resourceInfos)
	if err != nil {
		logger.Error("ResourceLogic FindAll error:", err)
		return
	}

	total = self.Count(ctx, querystring, args...)

	uidSet := set.New(set.NonThreadSafe)
	for _, resourceInfo := range resourceInfos {
		uidSet.Add(resourceInfo.Uid)
	}

	usersMap := DefaultUser.FindUserInfos(ctx, set.IntSlice(uidSet))

	resources = make([]map[string]interface{}, len(resourceInfos))

	for i, resourceInfo := range resourceInfos {
		dest := make(map[string]interface{})

		structs.FillMap(resourceInfo.Resource, dest)
		structs.FillMap(resourceInfo.ResourceEx, dest)

		dest["user"] = usersMap[resourceInfo.Uid]

		// 链接的host
		if resourceInfo.Form == model.LinkForm {
			urlObj, err := url.Parse(resourceInfo.Url)
			if err == nil {
				dest["host"] = urlObj.Host
			}
		} else {
			dest["url"] = "/resources/" + strconv.Itoa(resourceInfo.Resource.Id)
		}

		resources[i] = dest
	}

	return
}

func (ResourceLogic) Count(ctx context.Context, querystring string, args ...interface{}) int64 {

	var (
		total int64
		err   error
	)
	if querystring == "" {
		total, err = db.MasterDB.Count(new(model.Resource))
	} else {
		total, err = db.MasterDB.Where(querystring, args...).Count(new(model.Resource))
	}

	if err != nil {
		logger.Error("ResourceLogic Count error:", err)
	}

	return total
}

// FindByCatid 获得某个分类的资源列表，分页
func (ResourceLogic) FindByCatid(ctx context.Context, paginator *Paginator, catid int) (resources []map[string]interface{}, total int64) {

	var (
		count         = paginator.PerPage()
		resourceInfos = make([]*model.ResourceInfo, 0)
	)

	err := db.MasterDB.Join("INNER", "resource_ex", "resource.id=resource_ex.id").Where("catid=?", catid).
		Desc("resource.mtime").Limit(count, paginator.Offset()).Find(&resourceInfos)
	if err != nil {
		logger.Error("ResourceLogic FindByCatid error:", err)
		return
	}

	total, err = db.MasterDB.Where("catid=?", catid).Count(new(model.Resource))
	if err != nil {
		logger.Error("ResourceLogic FindByCatid count error:", err)
		return
	}

	uidSet := set.New(set.NonThreadSafe)
	for _, resourceInfo := range resourceInfos {
		uidSet.Add(resourceInfo.Uid)
	}

	usersMap := DefaultUser.FindUserInfos(ctx, set.IntSlice(uidSet))

	resources = make([]map[string]interface{}, len(resourceInfos))

	for i, resourceInfo := range resourceInfos {
		dest := make(map[string]interface{})

		structs.FillMap(resourceInfo.Resource, dest)
		structs.FillMap(resourceInfo.ResourceEx, dest)

		dest["user"] = usersMap[resourceInfo.Uid]

		// 链接的host
		if resourceInfo.Form == model.LinkForm {
			urlObj, err := url.Parse(resourceInfo.Url)
			if err == nil {
				dest["host"] = urlObj.Host
			}
		} else {
			dest["url"] = "/resources/" + strconv.Itoa(resourceInfo.Resource.Id)
		}

		resources[i] = dest
	}

	return
}

// FindByIds 获取多个资源详细信息
func (ResourceLogic) FindByIds(ids []int) []*model.Resource {
	if len(ids) == 0 {
		return nil
	}
	resources := make([]*model.Resource, 0)
	err := db.MasterDB.In("id", ids).Find(&resources)
	if err != nil {
		logger.Error("ResourceLogic FindByIds error:%+v", err)
		return nil
	}
	return resources
}

func (ResourceLogic) findById(id int) *model.Resource {
	resource := &model.Resource{}
	_, err := db.MasterDB.Id(id).Get(resource)
	if err != nil {
		logger.Error("ResourceLogic findById error:%+v", err)
	}
	return resource
}

// findByIds 获取多个资源详细信息 包内使用
func (ResourceLogic) findByIds(ids []int) map[int]*model.Resource {
	if len(ids) == 0 {
		return nil
	}
	resources := make(map[int]*model.Resource)
	err := db.MasterDB.In("id", ids).Find(&resources)
	if err != nil {
		logger.Error("ResourceLogic FindByIds error:%+v", err)
		return nil
	}
	return resources
}

// 获得资源详细信息
func (ResourceLogic) FindById(ctx context.Context, id int) (resourceMap map[string]interface{}, comments []map[string]interface{}) {

	resourceInfo := &model.ResourceInfo{}
	_, err := db.MasterDB.Join("INNER", "resource_ex", "resource.id=resource_ex.id").Where("resource.id=?", id).Get(resourceInfo)
	if err != nil {
		logger.Error("ResourceLogic FindById error:", err)
		return
	}

	resource := &resourceInfo.Resource
	if resource.Id == 0 {
		logger.Error("ResourceLogic FindById get error:", err)
		return
	}

	resourceMap = make(map[string]interface{})
	structs.FillMap(resource, resourceMap)
	structs.FillMap(resourceInfo.ResourceEx, resourceMap)

	resourceMap["catname"] = GetCategoryName(resource.Catid)
	// 链接的host
	if resource.Form == model.LinkForm {
		urlObj, err := url.Parse(resource.Url)
		if err == nil {
			resourceMap["host"] = urlObj.Host
		}
	} else {
		resourceMap["url"] = "/resources/" + strconv.Itoa(resource.Id)
	}

	// 评论信息
	comments, ownerUser, _ := DefaultComment.FindObjComments(ctx, id, model.TypeResource, resource.Uid, 0)
	resourceMap["user"] = ownerUser
	return
}

// 获取单个 Resource 信息（用于编辑）
func (ResourceLogic) FindResource(ctx context.Context, id int) *model.Resource {

	resource := &model.Resource{}
	_, err := db.MasterDB.Id(id).Get(resource)
	if err != nil {
		logger.Error("ResourceLogic FindResource [%d] error：%s\n", id, err)
	}

	return resource
}

// 获得某个用户最近的资源
func (ResourceLogic) FindRecent(ctx context.Context, uid int) []*model.Resource {
	resourceList := make([]*model.Resource, 0)
	err := db.MasterDB.Where("uid=?", uid).Limit(5).OrderBy("id DESC").Find(&resourceList)
	if err != nil {
		logger.Error("resource logic FindRecent error:%+v", err)
		return nil
	}

	return resourceList
}

// getOwner 通过id获得资源的所有者
func (ResourceLogic) getOwner(id int) int {
	resource := &model.Resource{}
	_, err := db.MasterDB.Id(id).Get(resource)
	if err != nil {
		logger.Error("resource logic getOwner Error:%+v", err)
		return 0
	}
	return resource.Uid
}

// 资源评论
type ResourceComment struct{}

// 更新该资源的评论信息
// cid：评论id；objid：被评论对象id；uid：评论者；cmttime：评论时间
func (self ResourceComment) UpdateComment(cid, objid, uid int, cmttime time.Time) {
	session := db.MasterDB.NewSession()
	defer session.Close()

	session.Begin()

	// 更新最后回复信息
	_, err := session.Table(new(model.Resource)).Id(objid).Update(map[string]interface{}{
		"lastreplyuid":  uid,
		"lastreplytime": cmttime,
	})
	if err != nil {
		logger.Error("更新最后回复人信息失败：%+v", err)
		session.Rollback()
		return
	}

	// 更新评论数（TODO：暂时每次都更新表）
	_, err = session.Id(objid).Incr("cmtnum", 1).Update(new(model.ResourceEx))
	if err != nil {
		logger.Error("更新资源评论数失败：%+v", err)
		session.Rollback()
		return
	}

	session.Commit()
}

func (self ResourceComment) String() string {
	return "resource"
}

// 实现 CommentObjecter 接口
func (self ResourceComment) SetObjinfo(ids []int, commentMap map[int][]*model.Comment) {
	resources := DefaultResource.FindByIds(ids)
	if len(resources) == 0 {
		return
	}

	for _, resource := range resources {
		objinfo := make(map[string]interface{})
		objinfo["title"] = resource.Title
		objinfo["uri"] = model.PathUrlMap[model.TypeResource]
		objinfo["type_name"] = model.TypeNameMap[model.TypeResource]

		for _, comment := range commentMap[resource.Id] {
			comment.Objinfo = objinfo
		}
	}
}

// 资源喜欢
type ResourceLike struct{}

// 更新该主题的喜欢数
// objid：被喜欢对象id；num: 喜欢数(负数表示取消喜欢)
func (self ResourceLike) UpdateLike(objid, num int) {
	// 更新喜欢数（TODO：暂时每次都更新表）
	_, err := db.MasterDB.Where("id=?", objid).Incr("likenum", num).Update(new(model.ResourceEx))
	if err != nil {
		logger.Error("更新资源喜欢数失败：%+v", err)
	}
}

func (self ResourceLike) String() string {
	return "resource"
}
