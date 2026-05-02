package mapper

import (
	"net/netip"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/state"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

func TestMapResponseBuilder_Basic(t *testing.T) {
	cfg := &types.Config{
		BaseDomain: "example.com",
		LogTail: types.LogTailConfig{
			Enabled: true,
		},
	}

	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	builder := m.NewMapResponseBuilder(nodeID)

	// Test basic builder creation
	assert.NotNil(t, builder)
	assert.Equal(t, nodeID, builder.nodeID)
	assert.NotNil(t, builder.resp)
	assert.False(t, builder.resp.KeepAlive)
	assert.NotNil(t, builder.resp.ControlTime)
	assert.WithinDuration(t, time.Now(), *builder.resp.ControlTime, time.Second)
}

func TestMapResponseBuilder_WithCapabilityVersion(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)
	capVer := tailcfg.CapabilityVersion(42)

	builder := m.NewMapResponseBuilder(nodeID).
		WithCapabilityVersion(capVer)

	assert.Equal(t, capVer, builder.capVer)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_WithDomain(t *testing.T) {
	domain := "test.example.com"
	cfg := &types.Config{
		ServerURL:  "https://test.example.com",
		BaseDomain: domain,
	}

	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	builder := m.NewMapResponseBuilder(nodeID).
		WithDomain()

	assert.Equal(t, domain, builder.resp.Domain)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_WithCollectServicesDisabled(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	builder := m.NewMapResponseBuilder(nodeID).
		WithCollectServicesDisabled()

	value, isSet := builder.resp.CollectServices.Get()
	assert.True(t, isSet)
	assert.False(t, value)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_WithDebugConfig(t *testing.T) {
	tests := []struct {
		name           string
		logTailEnabled bool
		expected       bool
	}{
		{
			name:           "LogTail enabled",
			logTailEnabled: true,
			expected:       false, // DisableLogTail should be false when LogTail is enabled
		},
		{
			name:           "LogTail disabled",
			logTailEnabled: false,
			expected:       true, // DisableLogTail should be true when LogTail is disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &types.Config{
				LogTail: types.LogTailConfig{
					Enabled: tt.logTailEnabled,
				},
			}
			mockState := &state.State{}
			m := &mapper{
				cfg:   cfg,
				state: mockState,
			}

			nodeID := types.NodeID(1)

			builder := m.NewMapResponseBuilder(nodeID).
				WithDebugConfig()

			require.NotNil(t, builder.resp.Debug)
			assert.Equal(t, tt.expected, builder.resp.Debug.DisableLogTail)
			assert.False(t, builder.hasErrors())
		})
	}
}

func TestMapResponseBuilder_WithDERPMapFiltersConnectivityZone(t *testing.T) {
	cfg, st, cleanup := setupConnectivityMapperTest(t)
	defer cleanup()

	cnNode := createConnectivityNode(t, st, "cn-node", []string{"tag:cn"}, "100.64.0.10", "203.0.113.10:41641")
	globalNode := createConnectivityNode(t, st, "global-node", []string{"tag:global"}, "100.64.0.20", "198.51.100.20:41641")

	m := &mapper{
		cfg:   cfg,
		state: st,
	}

	cnResp, err := m.NewMapResponseBuilder(cnNode.ID).
		WithDERPMap().
		Build()
	require.NoError(t, err)
	require.NotNil(t, cnResp.DERPMap)
	assert.ElementsMatch(t, []int{861}, derpRegionIDs(cnResp.DERPMap))

	globalResp, err := m.NewMapResponseBuilder(globalNode.ID).
		WithDERPMap().
		Build()
	require.NoError(t, err)
	require.NotNil(t, globalResp.DERPMap)
	assert.ElementsMatch(t, []int{901, 902}, derpRegionIDs(globalResp.DERPMap))
}

func TestMapResponseBuilder_WithPeersScrubsCrossZoneDirectCandidates(t *testing.T) {
	cfg, st, cleanup := setupConnectivityMapperTest(t)
	defer cleanup()

	cnNode := createConnectivityNode(t, st, "cn-node", []string{"tag:cn"}, "100.64.0.10", "203.0.113.10:41641")
	cnPeer := createConnectivityNode(t, st, "cn-peer", []string{"tag:cn"}, "100.64.0.11", "203.0.113.11:41641")
	globalPeer := createConnectivityNode(t, st, "global-peer", []string{"tag:global"}, "100.64.0.20", "198.51.100.20:41641")

	m := &mapper{
		cfg:   cfg,
		state: st,
	}

	resp, err := m.NewMapResponseBuilder(cnNode.ID).
		WithCapabilityVersion(1).
		WithPeers(views.SliceOf([]types.NodeView{cnPeer.View(), globalPeer.View()})).
		Build()
	require.NoError(t, err)
	require.Len(t, resp.Peers, 2)

	peers := map[tailcfg.NodeID]*tailcfg.Node{}
	for _, peer := range resp.Peers {
		peers[peer.ID] = peer
	}

	sameZonePeer := peers[tailcfg.NodeID(cnPeer.ID)]
	require.NotNil(t, sameZonePeer)
	assert.NotEmpty(t, sameZonePeer.Endpoints)
	assert.NotEqual(t, key.DiscoPublic{}, sameZonePeer.DiscoKey)

	crossZonePeer := peers[tailcfg.NodeID(globalPeer.ID)]
	require.NotNil(t, crossZonePeer)
	assert.Empty(t, crossZonePeer.Endpoints)
	assert.Equal(t, key.DiscoPublic{}, crossZonePeer.DiscoKey)
	assert.Equal(t, 861, crossZonePeer.HomeDERP)
	assert.Equal(t, "127.3.3.40:861", crossZonePeer.LegacyDERPString)
}

func TestMapResponseBuilder_WithPeerChangedPatch(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)
	changes := []*tailcfg.PeerChange{
		{
			NodeID:     123,
			DERPRegion: 1,
		},
		{
			NodeID:     456,
			DERPRegion: 2,
		},
	}

	builder := m.NewMapResponseBuilder(nodeID).
		WithPeerChangedPatch(changes)

	assert.Equal(t, changes, builder.resp.PeersChangedPatch)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_WithPeersRemoved(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)
	removedID1 := types.NodeID(123)
	removedID2 := types.NodeID(456)

	builder := m.NewMapResponseBuilder(nodeID).
		WithPeersRemoved(removedID1, removedID2)

	expected := []tailcfg.NodeID{
		removedID1.NodeID(),
		removedID2.NodeID(),
	}
	assert.Equal(t, expected, builder.resp.PeersRemoved)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_ErrorHandling(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	// Simulate an error in the builder
	builder := m.NewMapResponseBuilder(nodeID)
	builder.addError(assert.AnError)

	// All subsequent calls should continue to work and accumulate errors
	result := builder.
		WithDomain().
		WithCollectServicesDisabled().
		WithDebugConfig()

	assert.True(t, result.hasErrors())
	assert.Len(t, result.errs, 1)
	assert.Equal(t, assert.AnError, result.errs[0])

	// Build should return the error
	data, err := result.Build()
	assert.Nil(t, data)
	assert.Error(t, err)
}

func TestMapResponseBuilder_ChainedCalls(t *testing.T) {
	domain := "chained.example.com"
	cfg := &types.Config{
		ServerURL:  "https://chained.example.com",
		BaseDomain: domain,
		LogTail: types.LogTailConfig{
			Enabled: false,
		},
	}

	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)
	capVer := tailcfg.CapabilityVersion(99)

	builder := m.NewMapResponseBuilder(nodeID).
		WithCapabilityVersion(capVer).
		WithDomain().
		WithCollectServicesDisabled().
		WithDebugConfig()

	// Verify all fields are set correctly
	assert.Equal(t, capVer, builder.capVer)
	assert.Equal(t, domain, builder.resp.Domain)
	value, isSet := builder.resp.CollectServices.Get()
	assert.True(t, isSet)
	assert.False(t, value)
	assert.NotNil(t, builder.resp.Debug)
	assert.True(t, builder.resp.Debug.DisableLogTail)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_MultipleWithPeersRemoved(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)
	removedID1 := types.NodeID(100)
	removedID2 := types.NodeID(200)

	// Test calling WithPeersRemoved multiple times
	builder := m.NewMapResponseBuilder(nodeID).
		WithPeersRemoved(removedID1).
		WithPeersRemoved(removedID2)

	// Second call should overwrite the first
	expected := []tailcfg.NodeID{removedID2.NodeID()}
	assert.Equal(t, expected, builder.resp.PeersRemoved)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_EmptyPeerChangedPatch(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	builder := m.NewMapResponseBuilder(nodeID).
		WithPeerChangedPatch([]*tailcfg.PeerChange{})

	assert.Empty(t, builder.resp.PeersChangedPatch)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_NilPeerChangedPatch(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	builder := m.NewMapResponseBuilder(nodeID).
		WithPeerChangedPatch(nil)

	assert.Nil(t, builder.resp.PeersChangedPatch)
	assert.False(t, builder.hasErrors())
}

func TestMapResponseBuilder_MultipleErrors(t *testing.T) {
	cfg := &types.Config{}
	mockState := &state.State{}
	m := &mapper{
		cfg:   cfg,
		state: mockState,
	}

	nodeID := types.NodeID(1)

	// Create a builder and add multiple errors
	builder := m.NewMapResponseBuilder(nodeID)
	builder.addError(assert.AnError)
	builder.addError(assert.AnError)
	builder.addError(nil) // This should be ignored

	// All subsequent calls should continue to work
	result := builder.
		WithDomain().
		WithCollectServicesDisabled()

	assert.True(t, result.hasErrors())
	assert.Len(t, result.errs, 2) // nil error should be ignored

	// Build should return a multierr
	data, err := result.Build()
	require.Nil(t, data)
	require.Error(t, err)

	// The error should contain information about multiple errors
	assert.Contains(t, err.Error(), "multiple errors")
}

func setupConnectivityMapperTest(t testing.TB) (*types.Config, *state.State, func()) {
	t.Helper()

	prefixV4 := netip.MustParsePrefix("100.64.0.0/10")
	prefixV6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	cfg := &types.Config{
		Database: types.DatabaseConfig{
			Type: types.DatabaseSqlite,
			Sqlite: types.SqliteConfig{
				Path: t.TempDir() + "/headscale_test.db",
			},
		},
		PrefixV4:     &prefixV4,
		PrefixV6:     &prefixV6,
		IPAllocation: types.IPAllocationStrategySequential,
		BaseDomain:   "headscale.test",
		Policy: types.PolicyConfig{
			Mode: types.PolicyModeDB,
		},
		Taildrop: types.TaildropConfig{
			Enabled: true,
		},
		Connectivity: types.ConnectivityConfig{
			Zones: map[string]types.ConnectivityZoneConfig{
				"cn": {
					Tags:        []string{"tag:cn"},
					DERPRegions: []int{861},
				},
				"global": {
					Tags:        []string{"tag:global"},
					DERPRegions: []int{901, 902},
				},
			},
			CrossZoneDirect: types.CrossZoneDirectConfig{
				Enabled: false,
			},
		},
		Tuning: types.Tuning{
			NodeStoreBatchSize:    state.TestBatchSize,
			NodeStoreBatchTimeout: state.TestBatchTimeout,
		},
	}

	st, err := state.NewState(cfg)
	require.NoError(t, err)

	st.SetDERPMap(&tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			861: {RegionID: 861, RegionCode: "cn", RegionName: "China"},
			901: {RegionID: 901, RegionCode: "hk", RegionName: "Hong Kong"},
			902: {RegionID: 902, RegionCode: "jp", RegionName: "Japan"},
		},
	})

	return cfg, st, func() {
		require.NoError(t, st.Close())
	}
}

func createConnectivityNode(
	t testing.TB,
	st *state.State,
	hostname string,
	tags []string,
	tailnetIP string,
	endpoint string,
) *types.Node {
	t.Helper()

	user := st.CreateUserForTest(hostname + "-user")
	node := st.CreateRegisteredNodeForTest(user, hostname)
	node.Tags = tags
	node.IPv4 = ptr(netip.MustParseAddr(tailnetIP))
	node.Endpoints = []netip.AddrPort{netip.MustParseAddrPort(endpoint)}
	node.DiscoKey = key.NewDisco().Public()

	st.PutNodeInStoreForTest(*node)

	return node
}

func derpRegionIDs(dm *tailcfg.DERPMap) []int {
	ids := make([]int, 0, len(dm.Regions))
	for id := range dm.Regions {
		ids = append(ids, id)
	}

	return ids
}

func ptr[T any](v T) *T {
	return &v
}
