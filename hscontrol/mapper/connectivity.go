package mapper

import (
	"fmt"
	"slices"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

func zoneForNode(cfg types.ConnectivityConfig, node types.NodeView) (string, types.ConnectivityZoneConfig, bool) {
	if len(cfg.Zones) == 0 || !node.Valid() {
		return "", types.ConnectivityZoneConfig{}, false
	}

	names := make([]string, 0, len(cfg.Zones))
	for name := range cfg.Zones {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		zone := cfg.Zones[name]
		for _, tag := range zone.Tags {
			if node.HasTag(tag) {
				return name, zone, true
			}
		}
	}

	return "", types.ConnectivityZoneConfig{}, false
}

func filterDERPMapForNode(dm *tailcfg.DERPMap, cfg types.ConnectivityConfig, node types.NodeView) *tailcfg.DERPMap {
	if dm == nil {
		return nil
	}

	_, zone, ok := zoneForNode(cfg, node)
	if !ok || len(zone.DERPRegions) == 0 {
		return dm
	}

	regions := make(map[int]*tailcfg.DERPRegion, len(zone.DERPRegions))
	for _, regionID := range zone.DERPRegions {
		if region, ok := dm.Regions[regionID]; ok {
			regions[regionID] = region
		}
	}

	filtered := *dm
	filtered.Regions = regions

	return &filtered
}

func crossZonePeer(cfg types.ConnectivityConfig, requester, peer types.NodeView) (types.ConnectivityZoneConfig, bool) {
	requesterName, requesterZone, requesterOK := zoneForNode(cfg, requester)
	peerName, _, peerOK := zoneForNode(cfg, peer)

	if !requesterOK || !peerOK || requesterName == peerName {
		return types.ConnectivityZoneConfig{}, false
	}

	return requesterZone, true
}

func scrubCrossZonePeer(tn *tailcfg.Node, requesterZone types.ConnectivityZoneConfig) {
	tn.Endpoints = nil
	tn.DiscoKey = key.DiscoPublic{}

	if len(requesterZone.DERPRegions) > 0 {
		tn.HomeDERP = requesterZone.DERPRegions[0]
		tn.LegacyDERPString = fmt.Sprintf("127.3.3.40:%d", tn.HomeDERP)
	}
}
