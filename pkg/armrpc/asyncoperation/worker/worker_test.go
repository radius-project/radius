// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestDefaultOptions(t *testing.T) {
	worker := New(Options{}, nil, nil, nil)
	require.Equal(t, defaultDeduplicationDuration, worker.options.DeduplicationDuration)
	require.Equal(t, defaultMaxOperationRetryCount, worker.options.MaxOperationRetryCount)
	require.Equal(t, defaultMessageExtendMargin, worker.options.MessageExtendMargin)
	require.Equal(t, defaultMinMessageLockDuration, worker.options.MinMessageLockDuration)
	require.Equal(t, defaultMaxOperationConcurrency, worker.options.MaxOperationConcurrency)
}

func TestUpdateResourceState(t *testing.T) {
	updateStates := []struct {
		tc          string
		in          map[string]any
		updateState v1.ProvisioningState
		outErr      error
		callSave    bool
	}{
		{
			tc: "not found provisioningState",
			in: map[string]any{
				"name":       "env0",
				"properties": map[string]any{},
			},
			updateState: v1.ProvisioningStateAccepted,
			outErr:      nil,
			callSave:    true,
		},
		{
			tc: "not update state",
			in: map[string]any{
				"name":              "env0",
				"provisioningState": "Accepted",
				"properties":        map[string]any{},
			},
			updateState: v1.ProvisioningStateAccepted,
			outErr:      nil,
			callSave:    false,
		},
		{
			tc: "update state",
			in: map[string]any{
				"name":              "env0",
				"provisioningState": "Updating",
				"properties":        map[string]any{},
			},
			updateState: v1.ProvisioningStateAccepted,
			outErr:      nil,
			callSave:    true,
		},
	}

	for _, tt := range updateStates {
		t.Run(tt.tc, func(t *testing.T) {
			mctrl := gomock.NewController(t)
			defer mctrl.Finish()

			mStorageClient := store.NewMockStorageClient(mctrl)
			ctx := context.Background()

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Data: tt.in,
					}, nil
				})

			if tt.callSave {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, options ...store.SaveOptions) error {
						k := obj.Data.(map[string]any)
						require.Equal(t, k["provisioningState"].(string), string(tt.updateState))
						return nil
					})
			}

			err := updateResourceState(ctx, mStorageClient, "fakeid", tt.updateState)
			require.ErrorIs(t, err, tt.outErr)
		})
	}

}

func TestGetMessageExtendDuration(t *testing.T) {
	tests := []struct {
		in  time.Time
		out time.Duration
	}{
		{
			in:  time.Now().Add(defaultMessageExtendMargin),
			out: defaultMinMessageLockDuration,
		}, {
			in:  time.Now().Add(-defaultMessageExtendMargin),
			out: defaultMinMessageLockDuration,
		}, {
			in:  time.Now().Add(time.Duration(180) * time.Second),
			out: time.Duration(180)*time.Second - defaultMessageExtendMargin,
		},
	}

	for _, tt := range tests {
		worker := New(Options{}, nil, nil, nil)
		d := worker.getMessageExtendDuration(tt.in)
		require.Equal(t, tt.out, d.Round(time.Second))
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		err            error
		expectedArmErr v1.ErrorDetails
	}{
		{
			err:            conv.NewClientErrInvalidRequest("client error"),
			expectedArmErr: v1.ErrorDetails{Code: v1.CodeInvalid, Message: "client error"},
		},
		{
			err:            errors.New("internal error"),
			expectedArmErr: v1.ErrorDetails{Code: v1.CodeInternal, Message: "internal error"},
		},
	}

	for _, tt := range tests {
		armErr := extractError(tt.err)
		require.Equal(t, tt.expectedArmErr, armErr)
	}
}
