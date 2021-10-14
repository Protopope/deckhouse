/*
Copyright 2021 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scheduler

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/deckhouse/deckhouse/go_lib/set"
	"github.com/deckhouse/deckhouse/modules/500-upmeter/hooks/smokemini/internal/snapshot"
)

func NewNodeSelector(state State) NodeFilterPipe {
	return NodeFilterPipe{
		&filterByAvailability{},
		&filterByZone{state: state},
		&filterByMinSts{state: state},
		&nodeShuffler{},
	}
}

type NodeFilter interface {
	Filter([]snapshot.Node, string) []snapshot.Node
}

// NodeFilterPipe is the sequential wrapper for other node filters. A result of each filter is
// passed to the next one.  The filters do not share knowledge about each other. Filters are
// responsible to handle empty input their own way.
type NodeFilterPipe []NodeFilter

func (pipe NodeFilterPipe) Filter(nodes []snapshot.Node, x string) []snapshot.Node {
	for _, filter := range pipe {
		nodes = filter.Filter(nodes, x)
	}
	return nodes
}

// filterByAvailability filters nodes that are available for scheduling
type filterByAvailability struct{}

func (f *filterByAvailability) Filter(nodes []snapshot.Node, _ string) []snapshot.Node {
	return applyFilter(nodes, func(node snapshot.Node) bool {
		return node.Schedulable
	})
}

// filterByAvailability chooses a zone and returns nodes from that zone
type filterByZone struct {
	state State
}

func (f *filterByZone) Filter(nodes []snapshot.Node, x string) []snapshot.Node {
	if len(nodes) == 0 {
		return nodes
	}

	zone := f.selectZone(nodes, x)

	return applyFilter(nodes, func(node snapshot.Node) bool {
		return node.Zone == zone
	})
}

func (f *filterByZone) selectZone(nodes []snapshot.Node, x string) string {
	zones := f.collect(nodes, spread)
	return f.choose(zones, f.state[x])
}

func (f *filterByZone) choose(zones []*zoneStats, sts *XState) string {
	if len(zones) == 1 {
		return zones[0].name
	}

	// Check whether sts should change the zone
	var starvingZone *zoneStats
	for _, zone := range zones {
		if sts.Zone == zone.name && zone.demand() >= 0 {
			// The zone has no extra sts. This sts shouldn't leave the zone.
			return sts.Zone
		}
		if starvingZone == nil && zone.starving() {
			// Use first found starving zone
			starvingZone = zone
		}
	}

	if starvingZone != nil {
		return starvingZone.name
	}

	// Current zone
	return sts.Zone
}

func (f *filterByZone) collect(nodes []snapshot.Node, spread func(int, []int) []int) []*zoneStats {
	zonesByName := map[string]*zoneStats{}

	// Count nodes in zones
	for _, node := range nodes {
		if _, ok := zonesByName[node.Zone]; !ok {
			zonesByName[node.Zone] = &zoneStats{name: node.Zone}
		}
		zonesByName[node.Zone].nodes++
	}

	// Count sts in zones
	for _, sts := range f.state {
		if !sts.scheduled() {
			continue
		}
		if _, ok := zonesByName[sts.Zone]; !ok {
			// If a zone is not deduced from nodes, it is out of consideration.
			continue
		}
		zonesByName[sts.Zone].sts++
	}

	// The sorting is required to have stable sts distribution ordering. Otherwise, within
	// similar zones, sts would migrate from time to time because `spread` can have prioritized
	// direction.
	zones := make([]*zoneStats, 0)
	for _, z := range zonesByName {
		zones = append(zones, z)
	}
	sort.Sort(byName(zones))

	// Count the distribution of wanted amount of sts per zone
	nodeBuckets := make([]int, len(zones))
	names := make([]string, len(zones))
	for i, z := range zones {
		nodeBuckets[i] = z.nodes
		names[i] = z.name

	}

	dist := spread(len(f.state), nodeBuckets)
	for i, want := range dist {
		zones[i].wantSts = want
	}

	return zones
}

type zoneStats struct {
	name    string
	nodes   int
	sts     int
	wantSts int
}

// demand is the amount of sts the node lacks
func (s *zoneStats) demand() int {
	return s.wantSts - s.sts
}

func (s *zoneStats) starving() bool {
	return s.demand() > 0
}

type byName []*zoneStats

func (s byName) Len() int {
	return len(s)
}

func (s byName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byName) Less(i, j int) bool {
	return s[i].name < s[j].name
}

// filterByMinSts returns nodes with minimal sts count
type filterByMinSts struct {
	state State
}

func (f *filterByMinSts) Filter(nodes []snapshot.Node, x string) []snapshot.Node {
	destinations := f.selectNodes(nodes, x)
	return applyFilter(nodes, func(node snapshot.Node) bool {
		return destinations.Has(node.Name)
	})
}

func (f *filterByMinSts) selectNodes(nodes []snapshot.Node, x string) set.Set {
	var (
		stsPerNode  = map[string]int{}
		currentNode = f.state[x].Node
	)

	// Collect nodes of interest
	for _, n := range nodes {
		if n.Name == currentNode {
			// Our goal is to probe new node if possible
			continue
		}
		stsPerNode[n.Name] = 0
	}

	if len(stsPerNode) == 0 {
		// No new nodes to consider
		return set.New(currentNode)
	}

	// Count sts, skip current one from consideration
	for _, sts := range f.state {
		if sts.Node == "" || sts.Node == currentNode {
			continue
		}
		stsPerNode[sts.Node]++
	}

	dests := selectKeysByMinValue(stsPerNode)

	return set.New(dests...)
}

// applyFilter implements the boilerplate of filtering a Node slice
func applyFilter(nodes []snapshot.Node, filter func(snapshot.Node) bool) []snapshot.Node {
	filtered := make([]snapshot.Node, 0)
	for _, node := range nodes {
		if !filter(node) {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

// nodeShuffler shuffles the list of nodes, and does not filter any of
type nodeShuffler struct{}

func (f nodeShuffler) Filter(nodes []snapshot.Node, _ string) []snapshot.Node {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
	return nodes
}

func selectKeysByMinValue(kv map[string]int) []string {
	// Find minimum value
	min := math.MaxInt32
	for _, v := range kv {
		if min > v {
			min = v
		}
	}

	// Collect keys
	ks := make([]string, 0)
	for k, v := range kv {
		if v == min {
			ks = append(ks, k)
		}
	}

	return ks
}

// Spread calculates the distribution of the total number among buckets. When a demand becomes zero,
// it is ignored, since the bucket capacity is over. The only exception is when all demands are the
// same regardless their value. In this case the priority information is lost, and we treat all of
// them as valid and equal.
func spread(total int, buckets []int) []int {
	size := len(buckets)
	dist := make([]int, size)

	if size == 0 || total <= 0 {
		return dist
	}

	demands := make([]int, len(buckets))
	copy(demands, buckets)

Outer:
	for {

		min, eq := minAndAllEqual(demands)

		// Redistribute numbers per buckets
		for i := range demands {
			if demands[i] <= 0 && !eq {
				continue
			}

			dist[i]++
			total--
			if total == 0 {
				break Outer
			}

			if !eq {
				// The distribution is controlled by making demand <= zero
				demands[i] -= min
			}
		}
	}

	return dist
}

// minAndAllEqual returns the minimal element from slice and whether all elements are equal
func minAndAllEqual(xs []int) (int, bool) {
	min := math.MaxInt32
	allEq := true

	first := xs[0]
	for _, x := range xs {
		if min > x && x > 0 {
			min = x
		}
		allEq = allEq && first == x
	}

	return min, allEq
}
