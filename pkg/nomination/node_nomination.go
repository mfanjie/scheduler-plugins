/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nomination

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Nomination is a score plugin that favors nodes based on their nomiatable
// resources.
type Nomination struct {
	handle framework.Handle
	nominationScorer
}

// nominationScorer contains information to calculate resource allocation score.
type nominationScorer struct {
	Name string
}

var PlanNodeAnnotation = "migration.gocrane.io/plan"

var _ = framework.ScorePlugin(&Nomination{})
var _ = framework.PreBindPlugin(&Nomination{})

// Name is the name of the plugin used in the Registry and configurations.
const Name = "Nomination"

// Name returns name of the plugin. It is used in logs, etc.
func (nomi *Nomination) Name() string {
	return Name
}

// Score invoked at the score extension point.
func (nomi *Nomination) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := nomi.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	return nomi.score(pod, nodeInfo)
}

// score will use `scorer` function to calculate the score.
func (r *nominationScorer) score(
	pod *v1.Pod,
	nodeInfo *framework.NodeInfo) (int64, *framework.Status) {
	if len(pod.OwnerReferences) == 0 {
		return 0, nil
	}
	node := nodeInfo.Node()
	if node.Annotations == nil {
		return 0, nil
	}

	planStr := node.Annotations[PlanNodeAnnotation]
	if planStr == "" {
		return 0, nil
	}
	workloadCountMap := map[string]int{}
	err := json.Unmarshal([]byte(planStr), &workloadCountMap)
	if err != nil {
		klog.Errorf("Failed to ")
	}
	for _, owner := range pod.OwnerReferences {
		for key, value := range workloadCountMap {
			if fmt.Sprintf("%s/%s/%s", owner.Kind, pod.Namespace, owner.Name) == key {
				return int64(value * 100), nil
			}
		}
	}
	return 0, nil
}

// ScoreExtensions of the Score plugin.
func (nomi *Nomination) ScoreExtensions() framework.ScoreExtensions {
	// disable NormalizeScore as we don't need it
	return nomi
}

// New initializes a new plugin and returns it.
func New(obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &Nomination{
		handle: h,
		nominationScorer: nominationScorer{
			Name: Name,
		},
	}, nil
}

// NormalizeScore invoked after scoring all nodes.
func (nomi *Nomination) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	return nil
}

// PreBind is the functions invoked by the framework at "prebind" extension point.
func (nomi *Nomination) PreBind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := nomi.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	workloadCountMap := map[string]int{}
	annotations := nodeInfo.Node().Annotations
	if annotations == nil {
		return nil
	}
	planStr := annotations[PlanNodeAnnotation]
	if planStr == "" {
		return nil
	}
	err = json.Unmarshal([]byte(planStr), &workloadCountMap)
	if err != nil {
		klog.Errorf("Failed to ")
	}
	allowBind := false
	for _, owner := range pod.OwnerReferences {
		for key, value := range workloadCountMap {
			if fmt.Sprintf("%s/%s/%s", owner.Kind, pod.Namespace, owner.Name) == key {
				allowBind = true
				if value > 0 {
					workloadCountMap[key]--
				}
			}
		}
	}
	if !allowBind {
		return nil
	}
	b, err := json.Marshal(workloadCountMap)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	newAnnotationKey := PlanNodeAnnotation
	mergePatch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, newAnnotationKey, string(b)))
	_, err = nomi.handle.ClientSet().CoreV1().Nodes().Patch(context.TODO(), nodeName, types.MergePatchType, mergePatch, metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Error patching node: %v\n", err)
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to patch node: %v", err))
	}
	return nil
}
