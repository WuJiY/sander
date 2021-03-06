// Copyright 2016 The StudyGolang Authors. All rights reserved.
// Use of t source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author: polaris	polaris@studygolang.com

package model

import "time"

// Wiki .
type Wiki struct {
	Id      int       `json:"id" xorm:"pk autoincr"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	Uri     string    `json:"uri"`
	Uid     int       `json:"uid"`
	Cuid    string    `json:"cuid"`
	Viewnum int       `json:"viewnum"`
	Tags    string    `json:"tags"`
	Ctime   OftenTime `json:"ctime" xorm:"created"`
	Mtime   time.Time `json:"mtime" xorm:"<-"`

	Users map[int]*User `xorm:"-"`
}

// BeforeInsert .
func (t *Wiki) BeforeInsert() {
	if t.Tags == "" {
		t.Tags = AutoTag(t.Title, t.Content, 4)
	}
}
