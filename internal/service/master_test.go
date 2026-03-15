package service

import (
	"context"
	"testing"

	"go-dfs/api"
)

func TestAssignVolume_EdgeCases(t *testing.T) {
	s := NewMasterServer()

	t.Run("空集群请求", func(t *testing.T) {
		_, err := s.AssignVolume(context.Background(), &api.AssignVolumeRequest{Filename: "test.txt"})
		if err == nil {
			t.Error("当没有节点注册时，应该返回错误，但实际没有")
		}
	})

	t.Run("正常分配", func(t *testing.T) {
		// 模拟注册
		s.RegisterNode(context.Background(), &api.RegisterRequest{NodeId: "v1", Address: "1.1.1.1:50052"})

		resp, err := s.AssignVolume(context.Background(), &api.AssignVolumeRequest{Filename: "test.txt"})
		if err != nil || resp.Address != "1.1.1.1:50052" {
			t.Errorf("分配逻辑错误: %v", err)
		}
	})
}
