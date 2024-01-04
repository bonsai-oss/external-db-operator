//go:build integration

package integration_tests

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func kubectl(args ...string) *exec.Cmd {
	return exec.Command("kubectl", args...)
}

func mapIncludesKeys[T any](m map[string]T, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return false
		}
	}
	return true
}

func deployOperator(kubernetesApiClient *kubernetes.Clientset, name, provider, dsn string) error {
	operatorLabels := map[string]string{
		"app.kubernetes.io/name":     "external-db-operator",
		"app.kubernetes.io/instance": name,
	}

	image, operatorImageGiven := os.LookupEnv("OPERATOR_IMAGE")
	if !operatorImageGiven {
		image = "registry.fsrv.services/fsrvcorp/integration/external-db-operator:latest"
	}

	deploymentConfiguration := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-db-operator-" + name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: operatorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: operatorLabels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "external-db-operator-sa",
					RestartPolicy:      corev1.RestartPolicyAlways,
					InitContainers: []corev1.Container{
						{
							Name:  "wait-for-database",
							Image: "alpine:latest",
							Command: []string{
								"sh",
								"-c",
								"until getent hosts " + name + ".databases.svc.cluster.local; do echo 'Waiting for database connection...' && sleep 1; done",
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "external-db-operator",
							Image: image,
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 5,
								PeriodSeconds:       2,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/status",
										Port: intstr.FromInt32(8080),
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DATABASE_PROVIDER",
									Value: provider,
								},
								{
									Name:  "DATABASE_DSN",
									Value: dsn,
								},
								{
									Name:  "INSTANCE_NAME",
									Value: name,
								},
							},
						},
					},
				},
			},
		},
	}

	// check if deployment already exists
	_, getDeploymentError := kubernetesApiClient.AppsV1().Deployments("default").Get(context.Background(), deploymentConfiguration.Name, metav1.GetOptions{})
	if getDeploymentError == nil {
		// deployment already exists, delete it
		deleteDeploymentError := kubernetesApiClient.AppsV1().Deployments("default").Delete(context.Background(), deploymentConfiguration.Name, metav1.DeleteOptions{})
		if deleteDeploymentError != nil {
			return deleteDeploymentError
		}
	}

	_, operatorDeployError := kubernetesApiClient.AppsV1().Deployments("default").Create(context.Background(), deploymentConfiguration, metav1.CreateOptions{})

	return operatorDeployError
}

func TestDatabase(t *testing.T) {
	clientConfig, _ := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	kubernetesApiClient := kubernetes.NewForConfigOrDie(clientConfig)
	kubernetesDynamicClient := dynamic.NewForConfigOrDie(clientConfig)

	kubectl("apply", "-f", "../manifests/crd.yaml").Run()
	kubectl("apply", "-f", "../manifests/rbac.yaml").Run()

	databaseNamespace := "databases"
	kubectl("delete", "namespace", databaseNamespace).Run()
	kubectl("create", "namespace", databaseNamespace).Run()

	type check struct {
		packages []string
		command  string
	}

	postgresProviderCheck := check{
		packages: []string{"postgresql-client"},
		command:  "PGPASSWORD=$(cat /etc/db-credentials/password) psql -h $(cat /etc/db-credentials/host) -p $(cat /etc/db-credentials/port) -U $(cat /etc/db-credentials/username) -d $(cat /etc/db-credentials/database) -c 'SELECT 1'",
	}
	mysqlProviderCheck := check{
		packages: []string{"default-mysql-client"},
		command:  "mysql -h $(cat /etc/db-credentials/host) -P $(cat /etc/db-credentials/port) -u $(cat /etc/db-credentials/username) -p$(cat /etc/db-credentials/password) $(cat /etc/db-credentials/database) -e 'SELECT 1'",
	}

	for _, testCase := range []struct {
		name     string
		provider string
		dsn      string
		check    check
	}{
		{
			name:     "postgres",
			provider: "postgres",
			dsn:      "postgres://postgres:postgres@postgres.databases.svc.cluster.local:5432/postgres?sslmode=disable",
			check:    postgresProviderCheck,
		},
		{
			name:     "cockroach",
			provider: "postgres",
			dsn:      "postgres://postgres:postgres@cockroach.databases.svc.cluster.local:5432/postgres?sslmode=disable",
			check:    postgresProviderCheck,
		},
		{
			name:     "mysql",
			provider: "mysql",
			dsn:      "root:password@tcp(mysql.databases.svc.cluster.local:3306)/mysql?charset=utf8mb4&parseTime=True&loc=Local",
			check:    mysqlProviderCheck,
		},
		{
			name:     "mariadb",
			provider: "mysql",
			dsn:      "root:password@tcp(mariadb.databases.svc.cluster.local:3306)/mysql?charset=utf8mb4&parseTime=True&loc=Local",
			check:    mysqlProviderCheck,
		},
		{
			name:     "percona",
			provider: "mysql",
			dsn:      "root:password@tcp(percona.databases.svc.cluster.local:3306)/mysql?charset=utf8mb4&parseTime=True&loc=Local",
			check:    mysqlProviderCheck,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			databaseResourceNamespace := "test-namespace-" + testCase.name
			kubectl("delete", "namespace", databaseResourceNamespace).Run()
			kubectl("create", "namespace", databaseResourceNamespace).Run()

			// Deploy test database
			t.Run("deploy test database", func(t *testing.T) {
				if deployTestDatabaseOutput, deployTestDatabaseError := kubectl("apply", "-n", databaseNamespace, "-f", "../manifests/databases/"+testCase.name+".yaml").CombinedOutput(); deployTestDatabaseError != nil {
					t.Errorf("failed to deploy test database: %v \n %v", deployTestDatabaseError, string(deployTestDatabaseOutput))
					return
				} else {
					t.Log(string(deployTestDatabaseOutput))
				}
			})

			// Deploy operator
			t.Run("deploy operator", func(t *testing.T) {
				deployOperatorError := deployOperator(kubernetesApiClient, testCase.name, testCase.provider, testCase.dsn)
				if deployOperatorError != nil {
					t.Fatalf("failed to deploy operator: %v", deployOperatorError)
				}

				// Wait for operator to be ready
				if output, waitError := kubectl("wait", "--for=condition=Available", "--timeout=1m", "deployment/external-db-operator-"+testCase.name).CombinedOutput(); waitError != nil {
					t.Errorf("failed to wait for operator: %v \n %v", waitError, string(output))
					return
				} else {
					t.Log(string(output))
				}
			})

			// Create test database
			_, createTestDatabaseError := kubernetesDynamicClient.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
				Group:    "bonsai-oss.org",
				Version:  "v1",
				Resource: "databases",
			})).Namespace(databaseResourceNamespace).Create(context.Background(), &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "bonsai-oss.org/v1",
					"kind":       "Database",
					"metadata": map[string]interface{}{
						"name": "demo",
						"labels": map[string]interface{}{
							"bonsai-oss.org/external-db-operator": testCase.provider + "-" + testCase.name,
						},
					},
				},
			}, metav1.CreateOptions{})

			if createTestDatabaseError != nil {
				t.Errorf("failed to create test database: %v", createTestDatabaseError)
				return
			}

			time.Sleep(5 * time.Second)
			t.Run("check secret integrity", func(t *testing.T) {
				secret, secretGetError := kubernetesApiClient.
					CoreV1().
					Secrets(databaseResourceNamespace).
					Get(context.Background(), "edb-demo", metav1.GetOptions{})
				if secretGetError != nil {
					t.Errorf("failed to get secret: %v", secretGetError)
					return
				}

				if !mapIncludesKeys(secret.Data, "username", "password", "host", "port", "database") {
					t.Errorf("secret does not contain all required keys")
					return
				}
			})

			t.Run("check database integrity", func(t *testing.T) {
				_, jobError := kubernetesApiClient.BatchV1().Jobs(databaseResourceNamespace).Create(context.Background(), &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name: "db-integrity-check-" + uuid.NewString(),
					},
					Spec: batchv1.JobSpec{
						BackoffLimit: func() *int32 { i := int32(0); return &i }(),
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Volumes: []corev1.Volume{
									{
										Name: "db-credentials",
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "edb-demo",
											},
										},
									},
								},
								Containers: []corev1.Container{
									{
										Name:  "db-integrity-check",
										Image: "debian:sid",
										Command: []string{
											"bash",
											"-xc",
											"(apt update && apt install -y " + strings.Join(testCase.check.packages, " ") + ") >/dev/null 2>&1 && " + testCase.check.command,
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "db-credentials",
												MountPath: "/etc/db-credentials",
											},
										},
									},
								},
							},
						},
					},
				}, metav1.CreateOptions{})
				if jobError != nil {
					t.Errorf("failed to create job: %v", jobError)
					return
				}

				// check for job completion
				watcher, _ := kubernetesApiClient.BatchV1().Jobs(databaseResourceNamespace).Watch(context.Background(), metav1.ListOptions{
					Watch: true,
				})
				defer watcher.Stop()

				for event := range watcher.ResultChan() {
					job, _ := event.Object.(*batchv1.Job)
					if len(job.Status.Conditions) == 0 {
						continue
					}

					switch job.Status.Conditions[0].Type {
					case batchv1.JobComplete:
						return
					default:
						t.Errorf("job failed: %v", job.Status.Conditions[0].Message)
						return
					}
				}
			})
		})
	}
}
