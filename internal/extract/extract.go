// Package extract parses rendered Kubernetes manifests into the cloud-neutral
// model.ResourceModel: workloads (with replica bounds and per-container
// resources), persistent volumes, and LoadBalancer services.
package extract

import (
	"bufio"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/anomalyco/plimsoll/internal/model"
)

// hpaTarget identifies the workload an HPA scales, by kind and name.
type hpaTarget struct {
	kind string
	name string
}

// Extract decodes a concatenated multi-document manifest string into a
// ResourceModel. Unknown object kinds are ignored. A malformed document yields
// an error.
func Extract(manifests string) (model.ResourceModel, error) {
	docs, err := splitYAML(manifests)
	if err != nil {
		return model.ResourceModel{}, err
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode

	var (
		m    model.ResourceModel
		hpas = map[hpaTarget]model.ReplicaBounds{}
		// pending holds workloads before HPA bounds are resolved, keyed by index.
		pending []pendingWorkload
	)

	for _, doc := range docs {
		obj, _, err := decode([]byte(doc), nil, nil)
		if err != nil {
			// Tolerate documents whose kind isn't registered; fail on true garbage.
			if runtime.IsNotRegisteredError(err) {
				continue
			}
			return model.ResourceModel{}, fmt.Errorf("decoding manifest: %w", err)
		}

		switch o := obj.(type) {
		case *appsv1.Deployment:
			pending = append(pending, pendingWorkload{
				wl: model.Workload{
					Name: o.Name,
					Kind: model.KindDeployment,
					Pod:  podSpecFrom(o.Spec.Template.Spec),
				},
				static: replicasOrDefault(o.Spec.Replicas),
			})
		case *appsv1.StatefulSet:
			pending = append(pending, pendingWorkload{
				wl: model.Workload{
					Name: o.Name,
					Kind: model.KindStatefulSet,
					Pod:  podSpecFrom(o.Spec.Template.Spec),
				},
				static: replicasOrDefault(o.Spec.Replicas),
			})
			m.Volumes = append(m.Volumes, volumesFromClaimTemplates(o.Spec.VolumeClaimTemplates)...)
		case *appsv1.ReplicaSet:
			pending = append(pending, pendingWorkload{
				wl: model.Workload{
					Name: o.Name,
					Kind: model.KindReplicaSet,
					Pod:  podSpecFrom(o.Spec.Template.Spec),
				},
				static: replicasOrDefault(o.Spec.Replicas),
			})
		case *appsv1.DaemonSet:
			// A DaemonSet runs one pod per node; we record a single template pod
			// and let the packer expand it per node.
			pending = append(pending, pendingWorkload{
				wl: model.Workload{
					Name: o.Name,
					Kind: model.KindDaemonSet,
					Pod:  podSpecFrom(o.Spec.Template.Spec),
				},
				static: 1,
			})
		case *corev1.Service:
			if o.Spec.Type == corev1.ServiceTypeLoadBalancer {
				m.LoadBalancers = append(m.LoadBalancers, loadBalancerFrom(o))
			}
		case *corev1.PersistentVolumeClaim:
			m.Volumes = append(m.Volumes, volumeFromPVCSpec(o.Name, o.Spec))
		case *autoscalingv2.HorizontalPodAutoscaler:
			ref := o.Spec.ScaleTargetRef
			hpas[hpaTarget{kind: ref.Kind, name: ref.Name}] = boundsFromHPA(o)
		}
	}

	// Resolve replica bounds: HPA wins, else static (min==max).
	for _, p := range pending {
		w := p.wl
		if b, ok := hpas[hpaTarget{kind: string(w.Kind), name: w.Name}]; ok {
			w.Replicas = b
		} else {
			w.Replicas = model.ReplicaBounds{Min: p.static, Max: p.static}
		}
		m.Workloads = append(m.Workloads, w)
	}

	return m, nil
}

type pendingWorkload struct {
	wl     model.Workload
	static int
}

// splitYAML splits a multi-document YAML string on "---" separators, dropping
// empty/comment-only documents.
func splitYAML(s string) ([]string, error) {
	var docs []string
	var cur strings.Builder
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	flush := func() {
		doc := cur.String()
		cur.Reset()
		if strings.TrimSpace(stripComments(doc)) != "" {
			docs = append(docs, doc)
		}
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scanning manifests: %w", err)
	}
	flush()
	return docs, nil
}

func stripComments(doc string) string {
	var b strings.Builder
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func replicasOrDefault(r *int32) int {
	if r == nil {
		return 1
	}
	return int(*r)
}

func podSpecFrom(spec corev1.PodSpec) model.PodSpec {
	var ps model.PodSpec
	for _, c := range spec.Containers {
		ps.Containers = append(ps.Containers, model.Container{
			Name:       c.Name,
			CPURequest: c.Resources.Requests.Cpu().MilliValue(),
			MemRequest: c.Resources.Requests.Memory().Value(),
			CPULimit:   c.Resources.Limits.Cpu().MilliValue(),
			MemLimit:   c.Resources.Limits.Memory().Value(),
		})
	}
	return ps
}

func loadBalancerFrom(s *corev1.Service) model.LoadBalancer {
	lb := model.LoadBalancer{Name: s.Name}
	for _, p := range s.Spec.Ports {
		lb.Ports = append(lb.Ports, p.Port)
	}
	return lb
}

func volumeFromPVCSpec(name string, spec corev1.PersistentVolumeClaimSpec) model.Volume {
	v := model.Volume{Name: name}
	if q := spec.Resources.Requests.Storage(); q != nil {
		v.SizeBytes = q.Value()
	}
	if spec.StorageClassName != nil {
		v.StorageClass = *spec.StorageClassName
	}
	if len(spec.AccessModes) > 0 {
		v.AccessMode = string(spec.AccessModes[0])
	}
	return v
}

func volumesFromClaimTemplates(tmpls []corev1.PersistentVolumeClaim) []model.Volume {
	var vols []model.Volume
	for _, t := range tmpls {
		vols = append(vols, volumeFromPVCSpec(t.Name, t.Spec))
	}
	return vols
}

func boundsFromHPA(h *autoscalingv2.HorizontalPodAutoscaler) model.ReplicaBounds {
	min := 1
	if h.Spec.MinReplicas != nil {
		min = int(*h.Spec.MinReplicas)
	}
	return model.ReplicaBounds{Min: min, Max: int(h.Spec.MaxReplicas)}
}
