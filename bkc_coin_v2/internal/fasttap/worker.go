package fasttap

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"bkc_coin_v2/internal/db"

	"github.com/redis/go-redis/v9"
)

func (e *Engine) StartWorker(ctx context.Context) {
	if !e.Enabled() || e.DB == nil {
		return
	}
	// Create consumer group (idempotent).
	if err := e.Rdb.XGroupCreateMkStream(ctx, e.StreamKey, e.StreamGroup, "$").Err(); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "busygroup") {
			log.Printf("fasttap: XGROUP CREATE error: %v", err)
		}
	}
	for i := 0; i < e.WorkerCount; i++ {
		consumer := fmt.Sprintf("%s-%d", e.StreamConsumer, i+1)
		enableClaim := i == 0
		go e.workerLoop(ctx, consumer, enableClaim)
	}
}

func (e *Engine) workerLoop(ctx context.Context, consumer string, enableClaim bool) {
	nextClaimAt := time.Now().Add(e.ClaimEvery)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		res, err := e.Rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    e.StreamGroup,
			Consumer: consumer,
			Streams:  []string{e.StreamKey, ">"},
			Count:    e.ReadCount,
			Block:    e.ReadBlock,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				if enableClaim && time.Now().After(nextClaimAt) {
					e.claimPending(ctx, consumer)
					nextClaimAt = time.Now().Add(e.ClaimEvery)
				}
				continue
			}
			// transient
			log.Printf("fasttap: XREADGROUP error: %v", err)
			time.Sleep(750 * time.Millisecond)
			if enableClaim && time.Now().After(nextClaimAt) {
				e.claimPending(ctx, consumer)
				nextClaimAt = time.Now().Add(e.ClaimEvery)
			}
			continue
		}
		if len(res) == 0 {
			if enableClaim && time.Now().After(nextClaimAt) {
				e.claimPending(ctx, consumer)
				nextClaimAt = time.Now().Add(e.ClaimEvery)
			}
			continue
		}

		for _, st := range res {
			if len(st.Messages) == 0 {
				continue
			}
			if ok := e.processMessages(ctx, st.Messages); !ok {
				// Keep unacked entries to be retried by this or claimed by another consumer.
				time.Sleep(250 * time.Millisecond)
			}
		}

		if enableClaim && time.Now().After(nextClaimAt) {
			e.claimPending(ctx, consumer)
			nextClaimAt = time.Now().Add(e.ClaimEvery)
		}
	}
}

func (e *Engine) claimPending(ctx context.Context, consumer string) {
	start := "0-0"
	for round := 0; round < e.ClaimMaxRounds; round++ {
		msgs, next, err := e.Rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   e.StreamKey,
			Group:    e.StreamGroup,
			Consumer: consumer,
			MinIdle:  e.ClaimMinIdle,
			Start:    start,
			Count:    e.ClaimCount,
		}).Result()
		if err != nil {
			if err != redis.Nil {
				log.Printf("fasttap: XAUTOCLAIM error: %v", err)
			}
			return
		}
		if len(msgs) == 0 {
			return
		}
		_ = e.processMessages(ctx, msgs)
		if next == "" || next == start || next == "0-0" {
			return
		}
		start = next
	}
}

type queuedTap struct {
	id string
	ev db.TapEvent
}

func (e *Engine) processMessages(ctx context.Context, msgs []redis.XMessage) bool {
	if len(msgs) == 0 {
		return true
	}

	tapBatch := make([]queuedTap, 0, len(msgs))
	ackNow := make([]string, 0, len(msgs))

	for _, msg := range msgs {
		if msg.Values == nil {
			ackNow = append(ackNow, msg.ID)
			continue
		}
		kind := asString(msg.Values["kind"])
		if kind != "tap" {
			// Unknown kind: ack to avoid poison queue.
			ackNow = append(ackNow, msg.ID)
			continue
		}
		uid, _ := strconv.ParseInt(asString(msg.Values["uid"]), 10, 64)
		coins, _ := strconv.ParseInt(asString(msg.Values["coins"]), 10, 64)
		taps, _ := strconv.ParseInt(asString(msg.Values["taps"]), 10, 64)
		req, _ := strconv.ParseInt(asString(msg.Values["req"]), 10, 64)
		day := strings.TrimSpace(asString(msg.Values["day"]))
		if uid <= 0 || coins <= 0 || taps <= 0 || day == "" {
			ackNow = append(ackNow, msg.ID)
			continue
		}
		tapBatch = append(tapBatch, queuedTap{
			id: msg.ID,
			ev: db.TapEvent{
				EventID: msg.ID,
				UserID:  uid,
				Coins:   coins,
				Taps:    taps,
				Day:     day,
				Req:     req,
			},
		})
	}

	if len(ackNow) > 0 {
		_ = e.Rdb.XAck(ctx, e.StreamKey, e.StreamGroup, ackNow...).Err()
	}

	// Apply taps in chunks so one very large stream read does not produce oversized SQL packets.
	for i := 0; i < len(tapBatch); i += e.ApplyBatchSize {
		end := i + e.ApplyBatchSize
		if end > len(tapBatch) {
			end = len(tapBatch)
		}
		part := tapBatch[i:end]
		events := make([]db.TapEvent, 0, len(part))
		ackIDs := make([]string, 0, len(part))
		for _, x := range part {
			events = append(events, x.ev)
			ackIDs = append(ackIDs, x.id)
		}

		if err := e.DB.ApplyTapEvents(ctx, events); err != nil {
			log.Printf("fasttap: apply events error: %v", err)
			return false
		}
		if len(ackIDs) > 0 {
			if err := e.Rdb.XAck(ctx, e.StreamKey, e.StreamGroup, ackIDs...).Err(); err != nil {
				log.Printf("fasttap: XACK error: %v", err)
				return false
			}
		}
	}
	return true
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}
