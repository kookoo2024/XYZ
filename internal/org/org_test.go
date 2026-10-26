package org

import "testing"

func TestParseRoster(t *testing.T) {
	text := "学号\t姓名\t性别\r\n2025001\t张三\t男\n李四,2025002,女\n  王五  \n\n2025004 赵六\n"
	got := ParseRoster(text)
	if len(got) != 4 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	want := []Student{{Name: "张三", StudentNo: "2025001", Gender: "男"}, {Name: "李四", StudentNo: "2025002", Gender: "女"}, {Name: "王五"}, {Name: "赵六", StudentNo: "2025004"}}
	for i, w := range want {
		if got[i].Name != w.Name || got[i].StudentNo != w.StudentNo || got[i].Gender != w.Gender {
			t.Errorf("#%d got %+v want %+v", i, got[i], w)
		}
	}
}
