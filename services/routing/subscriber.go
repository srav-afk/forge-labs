package routing

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

func RunSubscriber(ctx context.Context, rdb *redis.Client, holder *SnapshotHolder, metrics *Metrics) {
	sub := rdb.Subscribe(ctx, Channel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var snap RoutingSnapshot
			if err := json.Unmarshal([]byte(msg.Payload), &snap); err != nil {
				if metrics != nil {
					metrics.IncDecodeError()
				}
				log.Printf("routing: bad snapshot payload: %v", err)
				continue
			}
			if !holder.StoreIfNewer(&snap) {
				continue
			}
		}
	}
}
