package discovery

import (
	"net"

	"github.com/hashicorp/serf/serf"
	"go.uber.org/zap"
)

// Membership manages cluster membership using HashiCorp Serf.
// It provides service discovery capabilities through a gossip protocol,
// automatically detecting when nodes join or leave the cluster.
//
// The Membership struct embeds configuration and maintains a Serf agent
// that communicates with other cluster members to propagate membership changes.
// When membership events occur, the configured Handler is called to take
// appropriate action (e.g., updating load balancer routes).
type Membership struct {
	Config                    // Embedded configuration for node settings
	handler Handler           // Handler for processing membership events (join/leave)
	serf    *serf.Serf        // Serf agent for gossip-based cluster membership
	events  chan serf.Event   // Channel for receiving Serf membership events
	logger  *zap.Logger       // Logger for membership-related events and errors
}

// New creates a new Membership instance for cluster discovery and management.
// It initializes a Serf agent that uses gossip protocol to maintain cluster membership.
//
// Parameters:
//   - handler: Interface for handling cluster membership events (join/leave)
//   - config: Configuration including node name, bind address, tags, and join addresses
//
// Returns a configured Membership instance ready to participate in the cluster,
// or an error if Serf initialization fails.
func New(handler Handler, config Config) (*Membership, error) {
	c := &Membership{
		Config:  config,
		handler: handler,
		logger:  zap.L().Named("membership"),
	}
	if err := c.setupSerf(); err != nil {
		return nil, err
	}
	return c, nil
}

// Config holds the configuration for Serf cluster membership.
type Config struct {
	NodeName       string            // Unique identifier for this node in the cluster
	BindAddr       string            // TCP address (IP:port) for Serf gossip communication
	Tags           map[string]string // Metadata shared with other cluster members (e.g., "rpc_addr")
	StartJoinAddrs []string          // Addresses of existing cluster members to join on startup
}

// setupSerf initializes the Serf cluster membership service.
// Serf uses a gossip protocol to maintain cluster membership and propagate events.
func (m *Membership) setupSerf() (err error) {
	// TCPアドレスを解析し、バインドするIPとポートを取得
	addr, err := net.ResolveTCPAddr("tcp", m.BindAddr)
	if err != nil {
		return err
	}

	// Serfのデフォルト設定を取得し、初期化
	config := serf.DefaultConfig()
	config.Init()

	// Memberlist（Serfの基盤となるゴシッププロトコル）の設定
	// このIPとポートでクラスターの他のノードと通信を行う
	config.MemberlistConfig.BindAddr = addr.IP.String()
	config.MemberlistConfig.BindPort = addr.Port

	// クラスターイベント（ノード参加/離脱）を受信するためのチャネルを作成
	m.events = make(chan serf.Event)
	config.EventCh = m.events

	// このノードのメタデータ（タグ）とノード名を設定
	// タグはクラスター内の他のノードと共有される情報（例：RPC address）
	config.Tags = m.Tags
	config.NodeName = m.Config.NodeName

	// Serfエージェントを作成。この時点でゴシッププロトコルが開始される
	m.serf, err = serf.Create(config)
	if err != nil {
		return err
	}

	// クラスターイベントを処理するgoroutineを開始
	go m.eventHandler()

	// 指定されたアドレスのクラスターに参加
	// Serfは参加したノードからクラスター全体の情報を学習する
	if m.StartJoinAddrs != nil {
		if _, err := m.serf.Join(m.StartJoinAddrs, true); err != nil {
			return err
		}
	}
	return nil
}

// eventHandler processes Serf cluster membership events in a background goroutine.
// Serf propagates membership changes (join/leave/fail) through gossip protocol to all nodes.
func (m *Membership) eventHandler() {
	// Serfからのイベントを継続的に監視
	for e := range m.events {
		switch e.EventType() {
		case serf.EventMemberJoin:
			// 新しいノードがクラスターに参加した際のイベント
			// ゴシッププロトコルにより、クラスター内の全ノードに伝播される
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue // 自分自身の参加イベントは無視
				}
				m.handleJoin(member)
			}
		case serf.EventMemberLeave, serf.EventMemberFailed:
			// ノードが正常に離脱、または障害で到達不能になった際のイベント
			// EventMemberLeave: 正常な離脱 (Leave()呼び出し)
			// EventMemberFailed: 障害による離脱 (タイムアウト等)
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					return // 自分自身の離脱イベントの場合は処理終了
				}
				m.handleLeave(member)
			}
		}
	}
}

func (m *Membership) handleJoin(member serf.Member) {
	if err := m.handler.Join(member.Name, member.Tags["rpc_addr"]); err != nil {
		m.logError(err, "failed to join", member)
	}
}

func (m *Membership) handleLeave(member serf.Member) {
	if err := m.handler.Leave(member.Name); err != nil {
		m.logError(err, "failed to leave", member)
	}
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) Members() []serf.Member {
	return m.serf.Members()
}

func (m *Membership) Leave() error {
	return m.serf.Leave()
}

func (m *Membership) logError(err error, msg string, member serf.Member) {
	m.logger.Error(
		msg,
		zap.Error(err),
		zap.String("name", member.Name),
		zap.String("rpc_addr", member.Tags["rpc_addr"]),
	)
}

type Handler interface {
	Join(name, addr string) error
	Leave(name string) error
}
