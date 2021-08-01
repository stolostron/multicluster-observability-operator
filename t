[1mdiff --git a/operators/multiclusterobservability/controllers/multiclusterobservability/multiclusterobservability_controller.go b/operators/multiclusterobservability/controllers/multiclusterobservability/multiclusterobservability_controller.go[m
[1mindex 4b4cfa8..0ec82bf 100644[m
[1m--- a/operators/multiclusterobservability/controllers/multiclusterobservability/multiclusterobservability_controller.go[m
[1m+++ b/operators/multiclusterobservability/controllers/multiclusterobservability/multiclusterobservability_controller.go[m
[36m@@ -8,7 +8,7 @@[m [mimport ([m
 	"fmt"[m
 	"os"[m
 	"reflect"[m
[31m-	"time"[m
[32m+[m	[32m//"time"[m
 [m
 	"github.com/go-logr/logr"[m
 	routev1 "github.com/openshift/api/route/v1"[m
[36m@@ -25,7 +25,7 @@[m [mimport ([m
 	"k8s.io/apimachinery/pkg/runtime/schema"[m
 	"k8s.io/apimachinery/pkg/types"[m
 	"k8s.io/apimachinery/pkg/util/intstr"[m
[31m-	"k8s.io/apimachinery/pkg/util/wait"[m
[32m+[m	[32m//"k8s.io/apimachinery/pkg/util/wait"[m
 	ctrl "sigs.k8s.io/controller-runtime"[m
 	"sigs.k8s.io/controller-runtime/pkg/builder"[m
 	"sigs.k8s.io/controller-runtime/pkg/client"[m
[36m@@ -611,33 +611,35 @@[m [mfunc updateStorageSizeChange(c client.Client, matchLabels map[string]string, com[m
 		}[m
 	}[m
 [m
[31m-	if os.Getenv("UNIT_TEST") != "true" {[m
[31m-		// wait FileSystemResizePending condition for all the updated PVC[m
[31m-		err = wait.Poll(6*time.Second, 120*time.Second, func() (done bool, err error) {[m
[31m-			isThereNotReady := false[m
[31m-			for _, pvcNamespaceName := range updatedPVCNamespaceNameList {[m
[31m-				pvc := &corev1.PersistentVolumeClaim{}[m
[31m-				err := c.Get(context.TODO(), pvcNamespaceName, pvc)[m
[31m-				if err != nil {[m
[31m-					return false, err[m
[31m-				}[m
[31m-				isResizePending := false[m
[31m-				for _, condition := range pvc.Status.Conditions {[m
[31m-					if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending {[m
[31m-						isResizePending = true[m
[31m-						break[m
[32m+[m	[32m/*[m
[32m+[m		[32mif os.Getenv("UNIT_TEST") != "true" {[m
[32m+[m			[32m// wait FileSystemResizePending condition for all the updated PVC[m
[32m+[m			[32merr = wait.Poll(6*time.Second, 120*time.Second, func() (done bool, err error) {[m
[32m+[m				[32misThereNotReady := false[m
[32m+[m				[32mfor _, pvcNamespaceName := range updatedPVCNamespaceNameList {[m
[32m+[m					[32mpvc := &corev1.PersistentVolumeClaim{}[m
[32m+[m					[32merr := c.Get(context.TODO(), pvcNamespaceName, pvc)[m
[32m+[m					[32mif err != nil {[m
[32m+[m						[32mreturn false, err[m
[32m+[m					[32m}[m
[32m+[m					[32misResizePending := false[m
[32m+[m					[32mfor _, condition := range pvc.Status.Conditions {[m
[32m+[m						[32mif condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending {[m
[32m+[m							[32misResizePending = true[m
[32m+[m							[32mbreak[m
[32m+[m						[32m}[m
[32m+[m					[32m}[m
[32m+[m					[32mif !isResizePending {[m
[32m+[m						[32misThereNotReady = true[m
 					}[m
 				}[m
[31m-				if !isResizePending {[m
[31m-					isThereNotReady = true[m
[31m-				}[m
[32m+[m				[32mreturn !isThereNotReady, nil[m
[32m+[m			[32m})[m
[32m+[m			[32mif err != nil {[m
[32m+[m				[32mreturn err[m
 			}[m
[31m-			return !isThereNotReady, nil[m
[31m-		})[m
[31m-		if err != nil {[m
[31m-			return err[m
 		}[m
[31m-	}[m
[32m+[m	[32m*/[m
 [m
 	// update sts[m
 	for index, sts := range stsList {[m
