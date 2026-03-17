package pool

import (
	"context"
	"testing"
	"time"
)

// mockServer 用于测试的 mock 服务地址
const mockServerAddr = "localhost:99999"

func TestNewGRPCPool(t *testing.T) {
	t.Run("无效配置", func(t *testing.T) {
		invalidConfig := &Config{
			MaxConn: 0,
		}
		_, err := NewGRPCPool(mockServerAddr, invalidConfig)
		if err != ErrInvalidConfig {
			t.Errorf("期望 ErrInvalidConfig, 实际 %v", err)
		}
	})

	t.Run("默认配置", func(t *testing.T) {
		config := DefaultConfig()
		if config.InitialConn != 2 {
			t.Error("默认 InitialConn 应为 2")
		}
		if config.MaxConn != 10 {
			t.Error("默认 MaxConn 应为 10")
		}
	})
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "有效配置",
			config: &Config{
				MaxConn:     10,
				InitialConn: 2,
				MaxIdle:     5,
			},
			wantErr: false,
		},
		{
			name: "MaxConn 为 0",
			config: &Config{
				MaxConn: 0,
			},
			wantErr: true,
		},
		{
			name: "InitialConn 大于 MaxConn",
			config: &Config{
				MaxConn:     5,
				InitialConn: 10,
			},
			wantErr: false, // 会被自动调整
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.config.InitialConn > tt.config.MaxConn {
				t.Error("InitialConn 应被调整为不超过 MaxConn")
			}
		})
	}
}

func TestGRPCPoolStats(t *testing.T) {
	// 创建一个最小化的池进行测试
	pool := &GRPCPool{
		address:   mockServerAddr,
		config:    DefaultConfig(),
		available: make(chan *PooledConn, 10),
		conns:     make([]*PooledConn, 0),
	}

	// 添加一些 mock 连接
	now := time.Now()
	conn1 := &PooledConn{createdAt: now, lastUsedAt: now, inUse: 1}
	conn2 := &PooledConn{createdAt: now, lastUsedAt: now, inUse: 0}
	conn3 := &PooledConn{createdAt: now, lastUsedAt: now, inUse: 0}

	pool.conns = append(pool.conns, conn1, conn2, conn3)

	stats := pool.Stats()

	if stats["total"] != 3 {
		t.Errorf("total 应为 3, 实际 %v", stats["total"])
	}
	if stats["active"] != 1 {
		t.Errorf("active 应为 1, 实际 %v", stats["active"])
	}
	if stats["idle"] != 2 {
		t.Errorf("idle 应为 2, 实际 %v", stats["idle"])
	}
}

func TestGRPCPoolClose(t *testing.T) {
	pool := &GRPCPool{
		address:   mockServerAddr,
		config:    DefaultConfig(),
		available: make(chan *PooledConn, 10),
		conns:     make([]*PooledConn, 0),
		stopCh:    make(chan struct{}),
	}

	// 关闭
	err := pool.Close()
	if err != nil {
		t.Errorf("Close 不应返回错误: %v", err)
	}

	// 重复关闭不应出错
	err = pool.Close()
	if err != nil {
		t.Errorf("重复 Close 不应返回错误: %v", err)
	}

	// 关闭后 Get 应返回错误
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// 创建一个可用的池来测试关闭后行为
	pool2 := &GRPCPool{
		address:   mockServerAddr,
		config:    DefaultConfig(),
		available: make(chan *PooledConn, 10),
		conns:     make([]*PooledConn, 0),
		stopCh:    make(chan struct{}),
	}
	pool2.Close()

	_, err = pool2.Get(ctx)
	if err != ErrPoolClosed && err != context.DeadlineExceeded {
		t.Errorf("关闭后 Get 应返回 ErrPoolClosed, 实际 %v", err)
	}
}
