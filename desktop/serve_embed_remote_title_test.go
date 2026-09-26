package main

import "testing"

func TestRemoteSessionTitleFollowsDesktopPrecedence(t *testing.T) {
	cases := []struct {
		name string
		meta SessionMeta
		want string
	}{
		{"自定义名优先", SessionMeta{Title: "我的会话", TopicTitle: "话题名", Preview: "首条消息"}, "我的会话"},
		{"话题名压过首条消息", SessionMeta{TopicTitle: "新的会话", Preview: "你是怎么创建的"}, "新的会话"},
		{"都没有才用首条消息", SessionMeta{Preview: "首条消息"}, "首条消息"},
		{"全空保持空", SessionMeta{}, ""},
	}
	for _, c := range cases {
		if got := remoteSessionTitle(c.meta); got != c.want {
			t.Errorf("%s: remoteSessionTitle() = %q, want %q", c.name, got, c.want)
		}
	}
}
