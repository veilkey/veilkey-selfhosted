package api

import (
	"context"
	"net/http"

	chain "github.com/veilkey/veilkey-chain"
)

type txActorCtxKey struct{}

// TxActorMiddleware extracts actor info from HTTP requests and stores it in context.
// SubmitTx reads it back to stamp onto TxEnvelope.
func TxActorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := chain.TxActor{
			ActorType: "api",
			ActorID:   actorIDForRequest(r),
			Source:    r.Method + " " + r.URL.Path,
		}
		ctx := context.WithValue(r.Context(), txActorCtxKey{}, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// txActorFromCtx retrieves TxActor from context. Returns empty actor if not set.
func txActorFromCtx(ctx context.Context) chain.TxActor {
	if actor, ok := ctx.Value(txActorCtxKey{}).(chain.TxActor); ok {
		return actor
	}
	return chain.TxActor{ActorType: "system"}
}
