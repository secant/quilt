package stitch

import (
	"encoding/json"

	"github.com/robertkrimen/otto"
)

func parseContext(vm *otto.Otto) (evalCtx, error) {
	ctx := evalCtx{
		Containers: make(map[int]Container),
	}

	vmCtx, err := vm.Run("getDeployment()")
	if err != nil {
		return ctx, err
	}

	// Export() always returns `nil` as the error (it's only present for
	// backwards compatibility), so we can safely ignore it.
	exp, _ := vmCtx.Export()
	ctxStr, err := json.Marshal(exp)
	if err != nil {
		return ctx, err
	}
	err = json.Unmarshal(ctxStr, &ctx)
	return ctx, err
}
