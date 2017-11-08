package watch

import (
	"fmt"

	"github.com/Sirupsen/logrus"

	"github.com/zionwu/alertmanager-operator/api"
	"github.com/zionwu/alertmanager-operator/client/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	k8sapi "k8s.io/client-go/pkg/api"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"

	"k8s.io/client-go/tools/cache"
)

type statefulSetWatcher struct {
	informer cache.SharedIndexInformer
	cfg      *api.Config
	alert    *v1beta1.Alert
	stopc    chan struct{}
}

func newStatefulSetWatcher(alert *v1beta1.Alert, kclient kubernetes.Interface, cfg *api.Config) Watcher {
	rclient := kclient.Core().RESTClient()

	plw := cache.NewListWatchFromClient(rclient, "statefulsets", alert.Namespace, fields.OneTermEqualSelector(k8sapi.ObjectNameField, alert.TargetID))
	informer := cache.NewSharedIndexInformer(plw, &apiv1.Pod{}, resyncPeriod, cache.Indexers{})
	stopc := make(chan struct{})

	statefulSetWatcher := &statefulSetWatcher{
		informer: informer,
		alert:    alert,
		cfg:      cfg,
		stopc:    stopc,
	}

	return statefulSetWatcher
}

func (w *statefulSetWatcher) Watch() {
	w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.handleAdd,
		DeleteFunc: w.handleDelete,
		UpdateFunc: w.handleUpdate,
	})

	go w.informer.Run(w.stopc)
	//<-w.stopc
}

func (w *statefulSetWatcher) Stop() {
	close(w.stopc)
}

func (w *statefulSetWatcher) UpdateAlert(alert *v1beta1.Alert) {
	w.alert = alert
}

func (w *statefulSetWatcher) handleAdd(obj interface{}) {

}

func (w *statefulSetWatcher) handleDelete(obj interface{}) {

}

func (w *statefulSetWatcher) handleUpdate(oldObj, curObj interface{}) {
	oldStatefulSet, err := convertToStatefulSet(oldObj)
	if err != nil {
		logrus.Info("converting to StatefulSet object failed")
		return
	}

	curStatefulSet, err := convertToStatefulSet(curObj)
	if err != nil {
		logrus.Info("converting to StatefulSet object failed")
		return
	}

	if curStatefulSet.GetResourceVersion() != oldStatefulSet.GetResourceVersion() {
		logrus.Infof("different version, will not check node status")
		return
	}

	availableThreshold := (100 - w.alert.StatefulSetRule.UnavailablePercentage) * (*curStatefulSet.Spec.Replicas) / 100

	if curStatefulSet.Status.ReadyReplicas <= availableThreshold {
		logrus.Infof("%s is firing", w.alert.Description)
		err = sendAlert(w.cfg.ManagerUrl, w.alert)
		if err != nil {
			logrus.Errorf("Error while sending alert: %v", err)
		}
	}

}

func convertToStatefulSet(o interface{}) (*appv1beta1.StatefulSet, error) {

	ss, isStatefulSet := o.(*appv1beta1.StatefulSet)
	if !isStatefulSet {
		deletedState, ok := o.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil, fmt.Errorf("Received unexpected object: %v", o)
		}
		ss, ok = deletedState.Obj.(*appv1beta1.StatefulSet)
		if !ok {
			return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Pod object: %v", deletedState.Obj)
		}
	}

	return ss, nil
}