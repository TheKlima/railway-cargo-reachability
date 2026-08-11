package main

import (
	"reflect"
	"testing"
)

// cargoSet builds a map[int]bool from a list of original cargo IDs.
// Tests express expected results in terms of original cargo IDs; decodeResult
// converts the BitSet output of Solve into the same representation.
func cargoSet(items ...int) map[int]bool {
	s := make(map[int]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// decodeResult converts the BitSet-based output of Solve back to a
// map[StationID]map[int]bool keyed by original cargo IDs, so tests can use
// reflect.DeepEqual against plain cargoSet values without caring about the
// internal index representation.
func decodeResult(result map[StationID]BitSet, idxToCargo []int) map[StationID]map[int]bool {
	decoded := make(map[StationID]map[int]bool, len(result))
	for id, bs := range result {
		s := make(map[int]bool)
		for idx, cargo := range idxToCargo {
			if bs.Contains(idx) {
				s[cargo] = true
			}
		}
		decoded[id] = s
	}
	return decoded
}

func TestLinearPath(t *testing.T) {
	// 1 -> 2 -> 3
	stations := map[int]Station{
		1: {Unload: 0, Load: 10},
		2: {Unload: 0, Load: 20},
		3: {Unload: 0, Load: 30},
	}
	adj := map[int][]int{1: {2}, 2: {3}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(10),
		3: cargoSet(10, 20),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCycleWithUnloading(t *testing.T) {
	// 1 -> 2 -> 3 -> 1
	// each station unloads what the previous one loaded
	stations := map[int]Station{
		1: {Unload: 30, Load: 10},
		2: {Unload: 10, Load: 20},
		3: {Unload: 20, Load: 30},
	}
	adj := map[int][]int{1: {2}, 2: {3}, 3: {1}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(30),
		2: cargoSet(10),
		3: cargoSet(20),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBranchingPaths(t *testing.T) {
	// diamond: 1 -> 2, 1 -> 3, 2 -> 4, 3 -> 4; no unloading
	stations := map[int]Station{
		1: {Unload: 0, Load: 10},
		2: {Unload: 0, Load: 20},
		3: {Unload: 0, Load: 30},
		4: {Unload: 0, Load: 40},
	}
	adj := map[int][]int{1: {2, 3}, 2: {4}, 3: {4}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(10),
		3: cargoSet(10),
		4: cargoSet(10, 20, 30), // union of both routes
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnloadPreventsForwarding(t *testing.T) {
	// 1 -> 2 -> 3; station 2 unloads cargo 10 so it doesn't reach station 3
	stations := map[int]Station{
		1: {Unload: 99, Load: 10},
		2: {Unload: 10, Load: 20},
		3: {Unload: 99, Load: 30},
	}
	adj := map[int][]int{1: {2}, 2: {3}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(10),
		3: cargoSet(20), // 10 was unloaded at station 2
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBranchWithSelectiveUnload(t *testing.T) {
	// one branch removes cargo 10, the other keeps it;
	// cargo 10 still reaches station 4 via the path that preserves it
	stations := map[int]Station{
		1: {Unload: 99, Load: 10},
		2: {Unload: 10, Load: 20},
		3: {Unload: 99, Load: 30},
		4: {Unload: 99, Load: 40},
	}
	adj := map[int][]int{1: {2, 3}, 2: {4}, 3: {4}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(10),
		3: cargoSet(10),
		4: cargoSet(10, 20, 30),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCycleAccumulation(t *testing.T) {
	// 1 <-> 2; neither station unloads the other's cargo, so both accumulate both
	stations := map[int]Station{
		1: {Unload: 99, Load: 10},
		2: {Unload: 99, Load: 20},
	}
	adj := map[int][]int{1: {2}, 2: {1}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(10, 20),
		2: cargoSet(10, 20),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSelfLoop(t *testing.T) {
	stations := map[int]Station{
		1: {Unload: 0, Load: 10},
	}
	adj := map[int][]int{1: {1}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(10),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSameUnloadAndLoad(t *testing.T) {
	// station 2 unloads and loads the same type — it still departs with it
	stations := map[int]Station{
		1: {Unload: 0, Load: 5},
		2: {Unload: 5, Load: 5},
		3: {Unload: 0, Load: 0},
	}
	adj := map[int][]int{1: {2}, 2: {3}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(5),
		3: cargoSet(5),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnreachableStation(t *testing.T) {
	stations := map[int]Station{
		1: {Unload: 0, Load: 10},
		2: {Unload: 0, Load: 20},
		3: {Unload: 0, Load: 30},
	}
	adj := map[int][]int{1: {2}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(10),
		3: cargoSet(), // unreachable
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSingleStationNoTracks(t *testing.T) {
	stations := map[int]Station{
		1: {Unload: 10, Load: 20},
	}
	adj := map[int][]int{}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLongChainAccumulates(t *testing.T) {
	// 1 -> 2 -> 3 -> 4 -> 5; each station loads a unique type, nothing unloaded
	stations := map[int]Station{
		1: {Unload: 99, Load: 1},
		2: {Unload: 99, Load: 2},
		3: {Unload: 99, Load: 3},
		4: {Unload: 99, Load: 4},
		5: {Unload: 99, Load: 5},
	}
	adj := map[int][]int{1: {2}, 2: {3}, 3: {4}, 4: {5}}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(1),
		3: cargoSet(1, 2),
		4: cargoSet(1, 2, 3),
		5: cargoSet(1, 2, 3, 4),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComplexGraph(t *testing.T) {
	// 1 -> 2 -> 4 -> 6
	// 1 -> 3 -> 5 -> 6
	// 5 -> 5 (self-loop)
	stations := map[int]Station{
		1: {Unload: 99, Load: 1},
		2: {Unload: 99, Load: 2},
		3: {Unload: 1, Load: 3},
		4: {Unload: 99, Load: 4},
		5: {Unload: 99, Load: 5},
		6: {Unload: 99, Load: 6},
	}
	adj := map[int][]int{
		1: {2, 3},
		2: {4},
		3: {5},
		4: {6},
		5: {5, 6},
	}

	got := decodeResult(Solve(stations, adj, 1))
	want := map[int]map[int]bool{
		1: cargoSet(),
		2: cargoSet(1),
		3: cargoSet(1),
		4: cargoSet(1, 2),
		5: cargoSet(3, 5),
		6: cargoSet(1, 2, 3, 4, 5),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
