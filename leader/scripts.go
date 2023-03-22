package leader

import (
	"crypto/sha1"
	_ "embed"
	"encoding/hex"
)

//go:embed scripts/atomic_delete.lua
var atomicDeleteScript string

//go:embed scripts/atomic_renew.lua
var atomicRenewScript string

//go:embed scripts/atomic_set.lua
var atomicSetScript string

func hashScript(script string) string {
	sha := sha1.New()
	sha.Write([]byte(script))
	return hex.EncodeToString(sha.Sum(nil))
}

var (
	atomicDeleteSHA = hashScript(atomicDeleteScript)
	atomicRenewSHA  = hashScript(atomicRenewScript)
	atomicSetSHA    = hashScript(atomicSetScript)
)
