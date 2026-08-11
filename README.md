# Railway Cargo Reachability

Solution to a take-home screening task ("Task #1: railway cargo reachability")
for JetBrains' **Compiler Optimizations in Kotlin/Native** internship. The
task itself is a graph reachability problem, but it maps directly onto a
classic *compiler dataflow analysis* (think liveness or reaching-definitions),
so the solution below is written — and optimized — in those terms.

## Problem

A railway system consists of several stations connected by one-way tracks.

Each station is associated with **two cargo types**:

- One type that is **unloaded** when a train arrives.
- One type that is **loaded** before a train departs.

All trains start from the same **initial station**, carrying no cargo. Trains
may follow any valid route along the tracks.

Determine, for each station, which cargo types **might be on a train when it
arrives**. A cargo type is considered possible if there is **at least one
route** from the initial station that brings it to the station.

**Rules**

- When a train arrives at a station, first it **unloads** the cargo type
  this station consumes, then it **loads** the cargo type this station
  provides.
- Trains can carry multiple cargo types at the same time.
- Cargo types are abstract labels; the amount does not matter.

### Input Format

- The first line contains two integers: `S T` — the number of stations and
  tracks.
- The next `S` lines each contain three integers: `s c_unload c_load`, where
  - `s` — the id of a station
  - `c_unload` — the kind of goods unloaded at station `s`
  - `c_load` — the kind of goods loaded at station `s`
- The next `T` lines each contain two integers: `s_from s_to`, indicating a
  directed track from station `s_from` to station `s_to`.
- The last line contains a single integer `s_0`, the starting station.

### Output

For each station, sorted by id, print `id:` followed by the sorted list of
cargo ids that might be present on arrival (nothing after the colon if none
are reachable).

### Example

Input:

```
3 2
1 0 10
2 0 20
3 0 30
1 2
2 3
1
```

Output:

```
1:
2: 10
3: 10 20
```

## Approach

This is a forward dataflow analysis over a directed graph, structurally the
same as liveness or reaching-definitions analysis in a compiler:

- **Lattice**: for each station, the set of cargo types that might be on
  board is a subset of all cargo types, ordered by ⊆; it only grows, never
  shrinks.
- **Transfer function**: `OUT(station) = GEN(station) ∪ (IN(station) \
  KILL(station))`, where `GEN` is the cargo type the station always loads and
  `KILL` is the cargo type it unloads.
- **Meet operator**: union over all incoming edges — a cargo type reaches a
  station if *any* incoming route carries it.
- **Fixed point**: a FIFO worklist starts at the initial station and
  propagates `OUT` sets along edges; a neighbour is re-queued only when its
  `OUT` set actually grows. Because the lattice has finite height (at most
  `C` cargo types) and every update is monotonic, the algorithm always
  terminates at the unique least fixed point, regardless of cycles in the
  track graph.

See `Solve` in [main.go](main.go) for the implementation.

## Optimization journey: from hash sets to bit vectors

The first working version represented each station's cargo set as a
`map[int]bool`, and modelled a station as a pure function over that map:

```go
// v1 (naive): every set is a map[int]bool. Visiting a station allocates a
// brand-new map, copying every surviving cargo type one entry at a time.
func applyStation(arrival map[int]bool, station Station) map[int]bool {
    result := make(map[int]bool, len(arrival))
    for cargo := range arrival {
        if cargo != station.Unload {
            result[cargo] = true
        }
    }
    result[station.Load] = true
    return result
}
```

It's correct, but every station visit reallocates and rehashes the entire
set, even when the fixed-point iteration only ever *adds* bits. For `C`
distinct cargo types that's `O(C)` map operations (hashing + allocation) per
visit, repeated across the whole worklist.

The final version replaces every cargo set with a `BitSet` (`[]uint64`, one
bit per cargo index) — the same representation real compilers use for
liveness / reaching-definition bit vectors in dataflow passes:

- **Union is a word-wise bitwise OR** (`BitSet.Union`), so 64 cargo types are
  merged per CPU instruction instead of per-element hash-map inserts.
- **The whole transfer function collapses into one pass**: `UnionWithout`
  performs `OUT = GEN ∪ (IN \ KILL)` in place — masking out the unloaded
  cargo's bit while OR-ing in the incoming set — with zero temporary
  allocations.
- **Change detection is a plain word comparison** (`old != new`), so the
  worklist only re-enqueues a station when its departure set actually grew,
  instead of tracking a boolean by hand across nested loops.

See `BitSet`, `BitSet.Union`, and `BitSet.UnionWithout` in
[main.go](main.go) for the final implementation. The same technique —
representing dataflow facts as bit vectors instead of hash sets — is exactly
what makes passes like redundant static-initializer elimination or
array-bounds-check elimination fast enough to run on every compilation.

## Project structure

```
.
├── go.mod        — module definition
├── main.go       — Solve() dataflow engine, BitSet, and the CLI entry point
├── main_test.go  — unit tests: linear paths, cycles, branching, self-loops, ...
└── README.md
```

## Building & running

```sh
go build -o solver .
./solver < input.txt
```

or, without a separate build step:

```sh
go run . < input.txt
```

## Testing

```sh
go test ./... -v
```
