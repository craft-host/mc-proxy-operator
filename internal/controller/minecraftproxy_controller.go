package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	minecraftv1alpha1 "github.com/luisito666/mc-proxy-operator/api/v1alpha1"
	"github.com/luisito666/mc-proxy-operator/internal/proxy"
	"github.com/luisito666/mc-proxy-operator/internal/proxy/portmanager"
)

const finalizerName = "minecraft.miminecraftserver.com/route-cleanup"

type MinecraftProxyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	RouteTable      *proxy.RouteTable
	HandlerRegistry *proxy.HandlerRegistry
	PortManager     *portmanager.PortManager
}

// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies/finalizers,verbs=update

func (r *MinecraftProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var mcProxy minecraftv1alpha1.MinecraftProxy
	if err := r.Get(ctx, req.NamespacedName, &mcProxy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !mcProxy.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&mcProxy, finalizerName) {
			edition := string(mcProxy.Spec.Edition)

			if handler, ok := r.HandlerRegistry.Get(edition); ok {
				handler.RemoveRoute(mcProxy.Spec.Hostname, mcProxy.Spec.AssignedPort)
			}

			switch mcProxy.Spec.Edition {
			case minecraftv1alpha1.EditionJava:
				r.RouteTable.RemoveHostnameRoute(mcProxy.Spec.Hostname)
			case minecraftv1alpha1.EditionBedrock:
				r.RouteTable.RemovePortRoute(mcProxy.Spec.AssignedPort)
				r.PortManager.Release(mcProxy.Spec.AssignedPort)
			}

			logger.Info("ruta removida",
				"edition", edition,
				"hostname", mcProxy.Spec.Hostname,
				"port", mcProxy.Spec.AssignedPort,
			)

			controllerutil.RemoveFinalizer(&mcProxy, finalizerName)
			if err := r.Update(ctx, &mcProxy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&mcProxy, finalizerName) {
		controllerutil.AddFinalizer(&mcProxy, finalizerName)
		if err := r.Update(ctx, &mcProxy); err != nil {
			return ctrl.Result{}, err
		}
	}

	edition := string(mcProxy.Spec.Edition)
	handler, ok := r.HandlerRegistry.Get(edition)
	if !ok {
		logger.Error(nil, "edición no soportada", "edition", edition)
		meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
			Type:    "RouteConfigured",
			Status:  metav1.ConditionFalse,
			Reason:  "UnsupportedEdition",
			Message: "La edición '" + edition + "' no está soportada",
		})
		r.Status().Update(ctx, &mcProxy)
		return ctrl.Result{}, nil
	}

	backendNamespace := mcProxy.Spec.Backend.Namespace
	if backendNamespace == "" {
		backendNamespace = mcProxy.Namespace
	}

	servicePort := mcProxy.Spec.Backend.ServicePort
	if servicePort == 0 {
		servicePort = minecraftv1alpha1.DefaultServicePort(mcProxy.Spec.Edition)
	}

	assignedPort := mcProxy.Spec.AssignedPort
	if mcProxy.Spec.Edition == minecraftv1alpha1.EditionBedrock && assignedPort == 0 {
		var err error
		assignedPort, err = r.PortManager.Allocate(mcProxy.Spec.Hostname)
		if err != nil {
			logger.Error(err, "error asignando puerto Bedrock")
			meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
				Type:    "RouteConfigured",
				Status:  metav1.ConditionFalse,
				Reason:  "PortAllocationFailed",
				Message: err.Error(),
			})
			r.Status().Update(ctx, &mcProxy)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		mcProxy.Spec.AssignedPort = assignedPort
		if err := r.Update(ctx, &mcProxy); err != nil {
			r.PortManager.Release(assignedPort)
			return ctrl.Result{}, err
		}
	} else if mcProxy.Spec.Edition == minecraftv1alpha1.EditionBedrock && assignedPort > 0 {
		if err := r.PortManager.AllocateSpecific(assignedPort, mcProxy.Spec.Hostname); err != nil {
			logger.V(1).Info("puerto ya reservado o no disponible", "port", assignedPort, "error", err)
		}
	}

	backend := &proxy.Backend{
		ServiceName:  mcProxy.Spec.Backend.ServiceName,
		ServicePort:  servicePort,
		Namespace:    backendNamespace,
		MaxPlayers:   mcProxy.Spec.MaxPlayers,
		Edition:      edition,
		AssignedPort: assignedPort,
		Hostname:     mcProxy.Spec.Hostname,
	}

	switch mcProxy.Spec.Edition {
	case minecraftv1alpha1.EditionJava:
		r.RouteTable.SetHostnameRoute(mcProxy.Spec.Hostname, backend)
	case minecraftv1alpha1.EditionBedrock:
		r.RouteTable.SetPortRoute(assignedPort, backend)
	}

	if err := handler.AddRoute(mcProxy.Spec.Hostname, backend, assignedPort); err != nil {
		logger.Error(err, "error notificando al handler")
	}

	logger.Info("ruta configurada",
		"edition", edition,
		"hostname", mcProxy.Spec.Hostname,
		"backend", backend.Address(),
		"assignedPort", assignedPort,
	)

	mcProxy.Status.Ready = true
	mcProxy.Status.Edition = mcProxy.Spec.Edition
	mcProxy.Status.AssignedPort = assignedPort
	mcProxy.Status.ActiveConnections = backend.ActiveConnections.Load()

	meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
		Type:               "RouteConfigured",
		Status:             metav1.ConditionTrue,
		Reason:             "RouteActive",
		Message:            "Ruta activa vía " + edition + " handler",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &mcProxy); err != nil {
		logger.Error(err, "error actualizando status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *MinecraftProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&minecraftv1alpha1.MinecraftProxy{}).
		Complete(r)
}
