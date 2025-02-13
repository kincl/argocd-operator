// Copyright 2019 ArgoCD Operator Developers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package argocdexport

import (
	"context"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	batchv1b1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	argoprojv1a1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
)

// getArgoExportCommand will return the command for the ArgoCD export process.
func getArgoExportCommand(cr *argoprojv1a1.ArgoCDExport) []string {
	cmd := make([]string, 0)
	cmd = append(cmd, "uid_entrypoint.sh")
	cmd = append(cmd, "argocd-operator-util")
	cmd = append(cmd, "export")
	cmd = append(cmd, cr.Spec.Storage.Backend)
	return cmd
}

func getArgoExportContainerEnv(cr *argoprojv1a1.ArgoCDExport) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0)

	switch cr.Spec.Storage.Backend {
	case common.ArgoCDExportStorageBackendAWS:
		env = append(env, corev1.EnvVar{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: argoutil.FetchStorageSecretName(cr),
					},
					Key: "aws.access.key.id",
				},
			},
		})

		env = append(env, corev1.EnvVar{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: argoutil.FetchStorageSecretName(cr),
					},
					Key: "aws.secret.access.key",
				},
			},
		})
	}

	return env
}

// getArgoExportContainerImage will return the container image for ArgoCD.
func getArgoExportContainerImage(cr *argoprojv1a1.ArgoCDExport) string {
	img := cr.Spec.Image
	if len(img) <= 0 {
		img = common.ArgoCDDefaultExportJobImage
	}

	tag := cr.Spec.Version
	if len(tag) <= 0 {
		tag = common.ArgoCDDefaultExportJobVersion
	}

	return argoutil.CombineImageTag(img, tag)
}

// getArgoExportVolumeMounts will return the VolumneMounts for the given ArgoCDExport.
func getArgoExportVolumeMounts(cr *argoprojv1a1.ArgoCDExport) []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0)

	mounts = append(mounts, corev1.VolumeMount{
		Name:      "backup-storage",
		MountPath: "/backups",
	})

	mounts = append(mounts, corev1.VolumeMount{
		Name:      "secret-storage",
		MountPath: "/secrets",
	})

	return mounts
}

// getArgoSecretVolume will return the Secret Volume for the export process.
func getArgoSecretVolume(name string, cr *argoprojv1a1.ArgoCDExport) corev1.Volume {
	volume := corev1.Volume{
		Name: name,
	}

	volume.VolumeSource = corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: argoutil.FetchStorageSecretName(cr),
		},
	}

	return volume
}

// getArgoStorageVolume will return the storage Volume for the export process.
func getArgoStorageVolume(name string, cr *argoprojv1a1.ArgoCDExport) corev1.Volume {
	volume := corev1.Volume{
		Name: name,
	}

	if cr.Spec.Storage == nil || strings.ToLower(cr.Spec.Storage.Backend) == common.ArgoCDExportStorageBackendLocal {
		volume.VolumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: cr.Name,
			},
		}
	} else {
		volume.VolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}

	return volume
}

// newJob returns a new Job instance for the given ArgoCDExport.
func newJob(cr *argoprojv1a1.ArgoCDExport) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    common.DefaultLabels(cr.Name),
		},
	}
}

// newCronJob returns a new CronJob instance for the given ArgoCDExport.
func newCronJob(cr *argoprojv1a1.ArgoCDExport) *batchv1b1.CronJob {
	return &batchv1b1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    common.DefaultLabels(cr.Name),
		},
	}
}

func newExportPodSpec(cr *argoprojv1a1.ArgoCDExport) corev1.PodSpec {
	pod := corev1.PodSpec{}

	pod.Containers = []corev1.Container{{
		Command:         getArgoExportCommand(cr),
		Env:             getArgoExportContainerEnv(cr),
		Image:           getArgoExportContainerImage(cr),
		ImagePullPolicy: corev1.PullAlways,
		Name:            "argocd-export",
		VolumeMounts:    getArgoExportVolumeMounts(cr),
	}}

	pod.RestartPolicy = corev1.RestartPolicyOnFailure
	pod.ServiceAccountName = "argocd-application-controller"
	pod.Volumes = []corev1.Volume{
		getArgoStorageVolume("backup-storage", cr),
		getArgoSecretVolume("secret-storage", cr),
	}

	return pod
}

func newPodTemplateSpec(cr *argoprojv1a1.ArgoCDExport) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    common.DefaultLabels(cr.Name),
		},
		Spec: newExportPodSpec(cr),
	}
}

// reconcileCronJob will ensure that the CronJob for the ArgoCDExport is present.
func (r *ReconcileArgoCDExport) reconcileCronJob(cr *argoprojv1a1.ArgoCDExport) error {
	if cr.Spec.Storage == nil {
		return nil // Do nothing if storage options not set
	}

	cj := newCronJob(cr)
	if argoutil.IsObjectFound(r.Client, cr.Namespace, cj.Name, cj) {
		if *cr.Spec.Schedule != cj.Spec.Schedule {
			cj.Spec.Schedule = *cr.Spec.Schedule
			return r.Client.Update(context.TODO(), cj)
		}
		return nil
	}

	cj.Spec.Schedule = *cr.Spec.Schedule

	job := newJob(cr)
	job.Spec.Template = newPodTemplateSpec(cr)

	cj.Spec.JobTemplate.Spec = job.Spec

	if err := controllerutil.SetControllerReference(cr, cj, r.Scheme); err != nil {
		return err
	}
	return r.Client.Create(context.TODO(), cj)
}

// reconcileJob will ensure that the Job for the ArgoCDExport is present.
func (r *ReconcileArgoCDExport) reconcileJob(cr *argoprojv1a1.ArgoCDExport) error {
	if cr.Spec.Storage == nil {
		return nil // Do nothing if storage options not set
	}

	job := newJob(cr)
	if argoutil.IsObjectFound(r.Client, cr.Namespace, job.Name, job) {
		if job.Status.Succeeded > 0 && cr.Status.Phase != common.ArgoCDStatusCompleted {
			// Mark status Phase as Complete
			cr.Status.Phase = common.ArgoCDStatusCompleted
			return r.Client.Status().Update(context.TODO(), cr)
		}
		return nil // Job not complete, move along...
	}

	job.Spec.Template = newPodTemplateSpec(cr)

	if err := controllerutil.SetControllerReference(cr, job, r.Scheme); err != nil {
		return err
	}
	return r.Client.Create(context.TODO(), job)
}
