// The guide's first type of Bridge, laid out as an author's own module: its
// requirement on Oiko is the minimum Oiko it needs. The replace, which an
// author's go.mod doesn't have, builds it against this checkout's contract;
// the root's `make test` reaches it, since go test ./... skips nested modules.
module example.com/oiko-plug

go 1.27.1

require github.com/llehouerou/oiko v0.10.1

replace github.com/llehouerou/oiko => ../../..
