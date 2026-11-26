// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kolapsis/maintenant/internal/uid"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestDiscoverAll_WarnsAboutMisspelledOrInvalidAnnotations(t *testing.T) {
	ns := "shop-" + uid.New()[24:]
	annotated := func(name string, annotations map[string]string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: annotations}
	}
	logs := &lockedBuffer{}
	rt := withClient(&Runtime{
		logger:   slog.New(slog.NewTextHandler(logs, nil)),
		nsFilter: NewNamespaceFilter("", ""),
		prevCPU:  make(map[string]*cpuPrev),
		stopCh:   make(chan struct{}),
	}, fake.NewClientset(
		&appsv1.Deployment{ObjectMeta: annotated("api", map[string]string{"maintenant.ingore": "true"})},
		&corev1.Pod{ObjectMeta: annotated("debug", map[string]string{"maintenant.alert.severity": "Critical"})},
	))

	_, err := rt.discoverAll(context.Background())
	require.NoError(t, err)

	out := logs.String()
	assert.Contains(t, out, "label=maintenant.ingore")
	assert.Contains(t, out, ns+"/Deployment/api")
	assert.Contains(t, out, "label=maintenant.alert.severity")
	assert.Equal(t, 2, strings.Count(out, "level=WARN"), out)
}
