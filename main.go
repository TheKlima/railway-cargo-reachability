package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
)

type Station struct {
	Unload int
	Load   int
}

type StationID = int

// BitSet is a compact set of non-negative integer indices backed by a slice of
// 64-bit words. Each bit i represents whether index i is a member of the set.
//
// In compiler dataflow analyses (e.g. LLVM, GCC, Kotlin/Native IR passes),
// cargo/liveness/reaching-definition sets are almost always implemented as
// bit vectors rather than hash maps, because:
//   - Memory: one bit per element instead of ~50-100 bytes per hash map entry.
//   - Speed: union of two sets is a word-by-word bitwise OR — the CPU processes
//     64 elements per instruction, versus one hash lookup per element.
//   - Change detection: detecting whether a union added new bits is a single
//     comparison (oldWord != newWord), with no extra bookkeeping.
type BitSet []uint64

// NewBitSet allocates a BitSet large enough to hold n elements (indices 0..n-1).
func NewBitSet(n int) BitSet {
	return make(BitSet, (n/64)+1)
}

// Add sets bit i, marking index i as a member.
func (b BitSet) Add(i int) {
	b[i/64] |= 1 << (uint(i) % 64)
}

// Contains reports whether index i is a member.
func (b BitSet) Contains(i int) bool {
	return b[i/64]&(1<<(uint(i)%64)) != 0
}

// Union ORs every word of other into b (b |= other).
// Returns true if b changed (i.e. at least one new bit was set).
func (b BitSet) Union(other BitSet) bool {
	changed := false
	for i := range b {
		old := b[i]
		b[i] |= other[i]
		if b[i] != old {
			changed = true
		}
	}
	return changed
}

// UnionWithout ORs other into b while masking out bit exclude (b |= other &^ {exclude}).
// exclude == -1 is a sentinel meaning "no exclusion" (equivalent to plain Union).
// Returns true if b changed.
//
// This implements the dataflow transfer function OUT = GEN ∪ (IN ∖ KILL) in a
// single pass: KILL is the station's unload cargo, GEN is its load cargo
// (already in b), and IN is the incoming departure set.
func (b BitSet) UnionWithout(other BitSet, exclude int) bool {
	changed := false
	excludeWord := -1
	var excludeMask uint64
	if exclude >= 0 {
		excludeWord = exclude / 64
		excludeMask = 1 << (uint(exclude) % 64)
	}
	for i := range b {
		old := b[i]
		bits := other[i]
		if i == excludeWord {
			bits &^= excludeMask // clear the unloaded cargo's bit before OR-ing in
		}
		b[i] |= bits
		if b[i] != old {
			changed = true
		}
	}
	return changed
}

// Solve performs a forward dataflow analysis over the railway network to
// determine which cargo types might be present on a train upon arrival at
// each station.
//
// Parameters:
//   - stations: maps each station ID to its Station descriptor. Every station
//     ID that appears in adj must have an entry here.
//   - adj: directed adjacency list of the track graph; adj[s] lists all
//     stations reachable from s in one hop.
//   - start_station: ID of the station where all trains begin carrying no cargo.
//
// Returns:
//   - arrival: map from station ID to a BitSet of cargo indices present on
//     arrival. Use idxToCargo to convert indices back to original cargo IDs.
//   - idxToCargo: slice mapping each cargo index (0-based) to its original
//     cargo ID as given in the input.
//
// The algorithm is a worklist-based fixed-point iteration. Cargo sets grow
// monotonically (union only), guaranteeing termination. Departure sets are
// mutated in-place. A neighbour is only re-enqueued when its departure BitSet
// actually gains new bits, i.e. when UnionWithout returns true.
func Solve(
	stations map[StationID]Station,
	adj map[StationID][]StationID,
	start_station StationID,
) (arrival map[StationID]BitSet, idxToCargo []int) {

	// Build a compact 0-based index for every cargo type that can ever travel
	// on a train. Only Load types are ever added to a train; Unload types that
	// were never loaded receive sentinel index -1 when used as an exclusion mask.
	cargoToIdx := make(map[int]int, len(stations))
	idxToCargo = make([]int, 0, len(stations))
	for _, st := range stations {
		if _, exists := cargoToIdx[st.Load]; !exists {
			cargoToIdx[st.Load] = len(idxToCargo)
			idxToCargo = append(idxToCargo, st.Load)
		}
	}
	C := len(idxToCargo)

	arrival = make(map[StationID]BitSet, len(stations))
	departure_cargo := make(map[StationID]BitSet, len(stations))
	reached_stations := make(map[StationID]struct{}, len(stations))

	for s := range stations {
		arrival[s] = NewBitSet(C)
		departure_cargo[s] = NewBitSet(C)
	}

	// The starting station is reached immediately; its own load type is the
	// only cargo the train carries when it first departs.
	reached_stations[start_station] = struct{}{}
	departure_cargo[start_station].Add(cargoToIdx[stations[start_station].Load])

	work_list := []StationID{start_station}                      // FIFO queue of stations to process
	is_in_work_list := map[StationID]struct{}{start_station: {}} // set of stations currently in the work list

	for len(work_list) > 0 {
		current_station := work_list[0]

		// remove current station from the work list
		work_list = work_list[1:]
		delete(is_in_work_list, current_station)

		// looping through destinations of the current station
		for _, next_station := range adj[current_station] {
			is_next_station_departure_cargo_changed := false

			// first visit: the station's own load type enters its departure set
			if _, already_reached := reached_stations[next_station]; !already_reached {
				reached_stations[next_station] = struct{}{}
				departure_cargo[next_station].Add(cargoToIdx[stations[next_station].Load])
				is_next_station_departure_cargo_changed = true
			}

			// Propagate all cargo from the current station's departure into the
			// next station's arrival. No exclusion here — the unload happens
			// after arrival (the train carries everything in when it pulls in).
			arrival[next_station].Union(departure_cargo[current_station])

			// Propagate surviving cargo into the next station's departure set,
			// masking out the cargo that gets unloaded at next_station.
			// UnionWithout returns true only if the departure set actually grew,
			// so we only re-enqueue when there is genuinely new outgoing cargo.
			unloadIdx := -1 // sentinel: unload type was never loaded, nothing to mask
			if idx, ok := cargoToIdx[stations[next_station].Unload]; ok {
				unloadIdx = idx
			}
			if departure_cargo[next_station].UnionWithout(departure_cargo[current_station], unloadIdx) {
				is_next_station_departure_cargo_changed = true
			}

			if _, queued := is_in_work_list[next_station]; is_next_station_departure_cargo_changed && !queued {
				work_list = append(work_list, next_station)
				is_in_work_list[next_station] = struct{}{}
			}
		}
	}

	return arrival, idxToCargo
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Split(bufio.ScanWords)

	nextInt := func() int {
		sc.Scan()
		v, _ := strconv.Atoi(sc.Text())
		return v
	}

	S := nextInt()
	T := nextInt()

	stations := make(map[StationID]Station, S)
	for i := 0; i < S; i++ {
		id := nextInt()
		stations[id] = Station{Unload: nextInt(), Load: nextInt()}
	}

	adj := make(map[StationID][]StationID, T)
	for i := 0; i < T; i++ {
		from := nextInt()
		adj[from] = append(adj[from], nextInt())
	}

	start := nextInt()
	result, idxToCargo := Solve(stations, adj, start)

	ids := make([]StationID, 0, len(stations))
	for id := range stations {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	for _, id := range ids {
		bs := result[id]
		cargos := make([]int, 0)
		for idx, cargo := range idxToCargo {
			if bs.Contains(idx) {
				cargos = append(cargos, cargo)
			}
		}
		sort.Ints(cargos)

		fmt.Fprintf(w, "%d:", id)
		for _, c := range cargos {
			fmt.Fprintf(w, " %d", c)
		}
		fmt.Fprintln(w)
	}
}
