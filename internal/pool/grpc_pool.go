package pool

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	ErrPoolClosed    = errors.New("连接池已关闭")
	ErrConnUnavailable = errors.New("无可用连接")
	ErrInvalidConfig = errors.New("无效的配置")
)

// Config 连接池配置
type Config struct {
	// 初始连接数
	InitialConn int
	// 最大连接数
	MaxConn int
	// 最大空闲连接数
	MaxIdle int
	// 连接超时
	ConnTimeout time.Duration
	// 连接最大空闲时间
	MaxIdleTime time.Duration
	// 连接最大生命周期
	MaxLifetime time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		InitialConn: 2,
		MaxConn:     10,
		MaxIdle:     5,
		ConnTimeout: 5 * time.Second,
		MaxIdleTime: 10 * time.Minute,
		MaxLifetime: 30 * time.Minute,
	}
}

// PooledConn 包装 grpc.ClientConn，增加池管理功能
type PooledConn struct {
	*grpc.ClientConn
	pool        *GRPCPool
	createdAt   time.Time
	lastUsedAt  time.Time
	inUse       int32 // 原子操作标记是否在使用
}

// Release 将连接归还到池中
func (c *PooledConn) Release() {
	if c.pool == nil || c.ClientConn == nil {
		return
	}
	atomic.StoreInt32(&c.inUse, 0)
	c.lastUsedAt = time.Now()
	c.pool.release(c)
}

// IsHealthy 检查连接是否健康
func (c *PooledConn) IsHealthy() bool {
	if c.ClientConn == nil {
		return false
	}
	state := c.ClientConn.GetState()
	return state == connectivity.Idle || state == connectivity.Ready
}

// GRPCPool gRPC 连接池
type GRPCPool struct {
	mu          sync.RWMutex
	address     string
	dialOptions []grpc.DialOption
	
	// 连接管理
	conns       []*PooledConn
	available   chan *PooledConn // 可用连接队列
	
	// 配置
	config      *Config
	
	// 状态
	closed      int32
	connCount   int32 // 当前连接数
	
	// 清理协程控制
	stopCh      chan struct{}
}

// NewGRPCPool 创建连接池
func NewGRPCPool(address string, config *Config, dialOptions ...grpc.DialOption) (*GRPCPool, error) {
	if config == nil {
		config = DefaultConfig()
	}
	
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	
	// 默认使用不安全连接
	if len(dialOptions) == 0 {
		dialOptions = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}
	
	pool := &GRPCPool{
		address:     address,
		dialOptions: dialOptions,
		config:      config,
		available:   make(chan *PooledConn, config.MaxConn),
		stopCh:      make(chan struct{}),
	}
	
	// 创建初始连接
	for i := 0; i < config.InitialConn; i++ {
		conn, err := pool.createConn()
		if err != nil {
			pool.Close()
			return nil, err
		}
		pool.conns = append(pool.conns, conn)
		pool.available <- conn
	}
	
	atomic.StoreInt32(&pool.connCount, int32(config.InitialConn))
	
	// 启动清理协程
	go pool.cleanup()
	
	log.Printf("[连接池] 创建成功: %s, 初始连接: %d", address, config.InitialConn)
	return pool, nil
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.MaxConn <= 0 {
		return ErrInvalidConfig
	}
	if config.InitialConn > config.MaxConn {
		config.InitialConn = config.MaxConn
	}
	if config.MaxIdle > config.MaxConn {
		config.MaxIdle = config.MaxConn
	}
	return nil
}

// createConn 创建新连接
func (p *GRPCPool) createConn() (*PooledConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnTimeout)
	defer cancel()
	
	conn, err := grpc.DialContext(ctx, p.address, p.dialOptions...)
	if err != nil {
		return nil, err
	}
	
	now := time.Now()
	return &PooledConn{
		ClientConn: conn,
		pool:       p,
		createdAt:  now,
		lastUsedAt: now,
		inUse:      0,
	}, nil
}

// Get 获取连接（阻塞直到获取到或超时）
func (p *GRPCPool) Get(ctx context.Context) (*PooledConn, error) {
	if atomic.LoadInt32(&p.closed) == 1 {
		return nil, ErrPoolClosed
	}
	
	select {
	case conn := <-p.available:
		if conn.IsHealthy() {
			atomic.StoreInt32(&conn.inUse, 1)
			return conn, nil
		}
		// 连接不健康，关闭并创建新连接
		p.removeConn(conn)
		return p.Get(ctx) // 递归重试
		
	case <-ctx.Done():
		return nil, ctx.Err()
		
	default:
		// 没有可用连接，尝试创建新连接
		if atomic.LoadInt32(&p.connCount) < int32(p.config.MaxConn) {
			conn, err := p.createConn()
			if err != nil {
				return nil, err
			}
			atomic.AddInt32(&p.connCount, 1)
			atomic.StoreInt32(&conn.inUse, 1)
			
			p.mu.Lock()
			p.conns = append(p.conns, conn)
			p.mu.Unlock()
			
			return conn, nil
		}
		
		// 达到最大连接数，等待可用连接
		select {
		case conn := <-p.available:
			if conn.IsHealthy() {
				atomic.StoreInt32(&conn.inUse, 1)
				return conn, nil
			}
			p.removeConn(conn)
			return p.Get(ctx)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// GetNonBlocking 非阻塞获取连接
func (p *GRPCPool) GetNonBlocking() (*PooledConn, error) {
	if atomic.LoadInt32(&p.closed) == 1 {
		return nil, ErrPoolClosed
	}
	
	select {
	case conn := <-p.available:
		if conn.IsHealthy() {
			atomic.StoreInt32(&conn.inUse, 1)
			return conn, nil
		}
		p.removeConn(conn)
		return nil, ErrConnUnavailable
	default:
		return nil, ErrConnUnavailable
	}
}

// release 归还连接到池中
func (p *GRPCPool) release(conn *PooledConn) {
	if atomic.LoadInt32(&p.closed) == 1 {
		p.removeConn(conn)
		return
	}
	
	// 非阻塞放入可用队列
	select {
	case p.available <- conn:
	default:
		// 队列满了，直接关闭连接
		p.removeConn(conn)
	}
}

// removeConn 移除并关闭连接
func (p *GRPCPool) removeConn(conn *PooledConn) {
	if conn == nil || conn.ClientConn == nil {
		return
	}
	
	conn.ClientConn.Close()
	atomic.AddInt32(&p.connCount, -1)
	
	p.mu.Lock()
	for i, c := range p.conns {
		if c == conn {
			p.conns = append(p.conns[:i], p.conns[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
}

// cleanup 定期清理过期连接
func (p *GRPCPool) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			p.cleanupIdleConns()
		case <-p.stopCh:
			return
		}
	}
}

// cleanupIdleConns 清理空闲连接
func (p *GRPCPool) cleanupIdleConns() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	now := time.Now()
	activeCount := 0
	
	for _, conn := range p.conns {
		if atomic.LoadInt32(&conn.inUse) == 1 {
			activeCount++
			continue
		}
		
		// 检查是否需要清理
		shouldRemove := false
		
		// 超过最大生命周期
		if now.Sub(conn.createdAt) > p.config.MaxLifetime {
			shouldRemove = true
		}
		
		// 超过最大空闲时间
		if now.Sub(conn.lastUsedAt) > p.config.MaxIdleTime {
			shouldRemove = true
		}
		
		// 连接不健康
		if !conn.IsHealthy() {
			shouldRemove = true
		}
		
		if shouldRemove {
			go p.removeConn(conn)
		}
	}
	
	// 如果连接数小于初始值，补充连接
	currentCount := atomic.LoadInt32(&p.connCount)
	if int(currentCount) < p.config.InitialConn {
		for i := 0; i < p.config.InitialConn-int(currentCount); i++ {
			go func() {
				conn, err := p.createConn()
				if err != nil {
					return
				}
				atomic.AddInt32(&p.connCount, 1)
				p.mu.Lock()
				p.conns = append(p.conns, conn)
				p.mu.Unlock()
				p.available <- conn
			}()
		}
	}
}

// Stats 返回连接池统计信息
func (p *GRPCPool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	activeCount := 0
	idleCount := 0
	
	for _, conn := range p.conns {
		if atomic.LoadInt32(&conn.inUse) == 1 {
			activeCount++
		} else {
			idleCount++
		}
	}
	
	return map[string]interface{}{
		"address":      p.address,
		"total":        len(p.conns),
		"active":       activeCount,
		"idle":         idleCount,
		"max":          p.config.MaxConn,
		"available_queue": len(p.available),
	}
}

// Close 关闭连接池
func (p *GRPCPool) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return nil // 已经关闭
	}
	
	close(p.stopCh)
	
	// 关闭所有连接
	p.mu.Lock()
	conns := make([]*PooledConn, len(p.conns))
	copy(conns, p.conns)
	p.conns = p.conns[:0]
	p.mu.Unlock()
	
	for _, conn := range conns {
		if conn.ClientConn != nil {
			conn.ClientConn.Close()
		}
	}
	
	close(p.available)
	log.Printf("[连接池] 已关闭: %s", p.address)
	return nil
}
