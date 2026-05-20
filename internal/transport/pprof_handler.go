package transport

// pprof_handler.go — on-demand profiling via gRPC (server → agent pull model).
//
// Flow:
//  1. API sends RequestProfilePayload through the ServerMessage.request_profile field.
//  2. Agent calls handleProfileRequest in a goroutine.
//  3. Agent collects the profile in-memory (no disk I/O) using runtime/pprof.
//  4. Agent sends the raw bytes back via NodeMessage.profile_data.
//  5. If ctx is cancelled before CPU sampling ends, StopCPUProfile is called
//     immediately, preventing goroutine leaks.

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	pb "autohost-agent/internal/grpc/nodepb"
)

// cpuProfileMu ensures only one CPU profile runs at a time across the process.
// sync.Mutex.TryLock is available since Go 1.18.
var cpuProfileMu sync.Mutex

// handleProfileRequest is called in its own goroutine for every
// ServerMessage_RequestProfile received on the Connect stream.
// It collects the requested profile, then sends one NodeMessage_ProfileData back.
func (c *GRPCClient) handleProfileRequest(
	ctx context.Context,
	req *pb.RequestProfilePayload,
	out chan<- *pb.NodeMessage,
) {
	requestID := req.GetRequestId()
	profileType := req.GetProfileType()

	data, collectErr := collectProfile(ctx, profileType, req.GetDurationSeconds())

	errMsg := ""
	if collectErr != nil {
		errMsg = collectErr.Error()
		log.Printf("⚠️  pprof [%s]: %v", requestID, collectErr)
	} else {
		log.Printf("✅ pprof [%s]: collected %d bytes (%s)", requestID, len(data), profileType)
	}

	msg := &pb.NodeMessage{
		Payload: &pb.NodeMessage_ProfileData{
			ProfileData: &pb.ProfileDataPayload{
				RequestId:   requestID,
				ProfileType: profileType,
				Data:        data,
				Error:       errMsg,
			},
		},
	}

	select {
	case out <- msg:
	case <-ctx.Done():
		log.Printf("⚠️  pprof [%s]: context cancelled before result could be sent", requestID)
	}
}

// collectProfile dispatches to the right sampler based on profileType.
func collectProfile(ctx context.Context, profileType pb.ProfileType, durationSec int32) ([]byte, error) {
	switch profileType {
	case pb.ProfileType_PROFILE_TYPE_CPU:
		return collectCPUProfile(ctx, durationSec)
	case pb.ProfileType_PROFILE_TYPE_HEAP:
		return collectHeapProfile()
	default:
		return nil, fmt.Errorf("unknown profile type: %v", profileType)
	}
}

// collectCPUProfile starts pprof CPU sampling, waits for durationSec (or ctx
// cancellation), stops the sampler, and returns the raw bytes.
//
// The mutex ensures only one CPU profile runs at a time; concurrent requests
// receive an immediate error rather than corrupted data.
func collectCPUProfile(ctx context.Context, durationSec int32) ([]byte, error) {
	if !cpuProfileMu.TryLock() {
		return nil, fmt.Errorf("a CPU profile is already in progress; try again later")
	}
	defer cpuProfileMu.Unlock()

	if durationSec <= 0 {
		durationSec = 30
	}

	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err != nil {
		return nil, fmt.Errorf("StartCPUProfile: %w", err)
	}

	// Wait for the sampling window or context cancellation (e.g. user closed Dashboard).
	select {
	case <-time.After(time.Duration(durationSec) * time.Second):
	case <-ctx.Done():
		log.Printf("⚠️  pprof CPU: context cancelled after %ds, stopping early", durationSec)
	}

	// StopCPUProfile must always be called to flush the sampler; never defer
	// before Start succeeds, which is already handled above.
	pprof.StopCPUProfile()
	return buf.Bytes(), nil
}

// collectHeapProfile forces a GC cycle (so the snapshot reflects live objects)
// then writes a heap/alloc profile to an in-memory buffer.
func collectHeapProfile() ([]byte, error) {
	// Force a full GC before snapshotting so the profile shows live heap only.
	runtime.GC()

	var buf bytes.Buffer
	if err := pprof.WriteHeapProfile(&buf); err != nil {
		return nil, fmt.Errorf("WriteHeapProfile: %w", err)
	}
	return buf.Bytes(), nil
}
