// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControlsMatchChecksExpectedControlsOnly(t *testing.T) {
	mine := bucketControl{BucketName: "skr-e2e-bucket", CanRead: true, CanWrite: true}
	other := bucketControl{BucketName: "other-bucket", CanRead: false, CanWrite: false}

	require.True(t, controlsMatch([]bucketControl{mine}, []bucketControl{mine}),
		"the expected control must match itself")
	require.True(t, controlsMatch([]bucketControl{mine, other}, []bucketControl{mine}),
		"unexpected controls without permissions must be ignored")
	require.False(t, controlsMatch([]bucketControl{other}, []bucketControl{mine}),
		"a missing expected control must not match")
	require.False(t, controlsMatch([]bucketControl{mine}, []bucketControl{mine, other}),
		"missing expected controls must not match")
	require.False(t, controlsMatch([]bucketControl{{BucketName: "skr-e2e-bucket", CanRead: true, CanWrite: false}},
		[]bucketControl{mine}),
		"changed permissions must not match")
	require.False(t, controlsMatch([]bucketControl{mine, {BucketName: "other-bucket", CanRead: true, CanWrite: true}},
		[]bucketControl{mine}),
		"unexpected permissions must not match")
}

func TestObjectStorageSitePlanPath(t *testing.T) {
	require.Equal(t, "objectstorage/isk01/bucket", objectStorageSitePlanPath("isk01", ""),
		"a standard plan path must not include the plan segment")
	require.Equal(t, "objectstorage/arc02/bucket/1t", objectStorageSitePlanPath("arc02", "1t"),
		"an archive plan path must include the plan segment")
}
