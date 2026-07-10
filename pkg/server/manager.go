package server

import (
	"fmt"
	"sync"
)

// TunnelManager 管理所有活跃的隧道会话
type TunnelManager struct {
	mu          sync.RWMutex
	tunnels     map[string]*TunnelSession // subdomain → session
	globalToken string
}

// NewTunnelManager 创建隧道管理器
func NewTunnelManager(globalToken string) *TunnelManager {
	return &TunnelManager{
		tunnels:     make(map[string]*TunnelSession),
		globalToken: globalToken,
	}
}

// Register 注册会话的所有子域名
// 先全量检查冲突，再逐一写入，保证原子性
func (m *TunnelManager) Register(session *TunnelSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 全量检查冲突
	for _, subdomain := range session.Subdomains {
		if existing, ok := m.tunnels[subdomain]; ok && existing != session {
			return fmt.Errorf("subdomain %q already taken", subdomain)
		}
	}

	// 逐一写入
	for _, subdomain := range session.Subdomains {
		m.tunnels[subdomain] = session
	}

	return nil
}

// Unregister 注销会话的所有子域名
func (m *TunnelManager) Unregister(session *TunnelSession) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, subdomain := range session.Subdomains {
		// 只删除指向该会话的条目
		if existing, ok := m.tunnels[subdomain]; ok && existing == session {
			delete(m.tunnels, subdomain)
		}
	}
}

// Lookup 查找子域名对应的隧道会话
func (m *TunnelManager) Lookup(subdomain string) *TunnelSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tunnels[subdomain]
}

// Count 返回当前活跃隧道数
func (m *TunnelManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tunnels)
}

// CheckConflict 检查子域名列表中是否有冲突，返回第一个冲突的子域名
func (m *TunnelManager) CheckConflict(subdomains []string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, subdomain := range subdomains {
		if _, ok := m.tunnels[subdomain]; ok {
			return subdomain
		}
	}
	return ""
}
