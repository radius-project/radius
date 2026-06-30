package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MaxHTTPProxyDeploymentTimeout = time.Minute * time.Duration(10)
	HTTPProxyConditionValid       = "Valid"
	HTTPProxyStatusInvalid        = "invalid"
	HTTPProxyStatusValid          = "valid"
)

type httpProxyWaiter struct {
	dynamicClientSet           dynamic.Interface
	httpProxyDeploymentTimeout time.Duration
	cacheResyncInterval        time.Duration
}

// NewHTTPProxyWaiter returns a new instance of HTTPProxyWaiter
func NewHTTPProxyWaiter(dynamicClientSet dynamic.Interface) *httpProxyWaiter {
	return &httpProxyWaiter{
		dynamicClientSet:           dynamicClientSet,
		httpProxyDeploymentTimeout: MaxHTTPProxyDeploymentTimeout,
		cacheResyncInterval:        DefaultCacheResyncInterval,
	}
}

func (handler *httpProxyWaiter) addDynamicEventHandler(ctx context.Context, informerFactory dynamicinformer.DynamicSharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkHTTPProxyStatus(ctx, informerFactory, item, doneCh)
		},
		UpdateFunc: func(_, newObj any) {
			handler.checkHTTPProxyStatus(ctx, informerFactory, item, doneCh)
		},
	})

	if err != nil {
		logger.Error(err, "failed to add event handler")
	}
}

// addEventHandler is not implemented for HTTPProxyWaiter
func (handler *httpProxyWaiter) addEventHandler(ctx context.Context, informerFactory informers.SharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error) {
}

func (handler *httpProxyWaiter) waitUntilReady(ctx context.Context, obj client.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("httpProxyName", obj.GetName(), "namespace", obj.GetNamespace())

	if isContourHTTPProxyRouteChild(obj) {
		logger.Info(fmt.Sprintf("Not validating the deployment of route HTTP proxy for %s", obj.GetName()))
		return nil
	}

	doneCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.httpProxyDeploymentTimeout)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	// Create dynamic informer for HTTPProxy
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(handler.dynamicClientSet, 0, obj.GetNamespace(), nil)
	httpProxyInformer := dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR)
	// Add event handlers to the http proxy informer
	handler.addDynamicEventHandler(ctx, dynamicInformerFactory, httpProxyInformer.Informer(), obj, doneCh)

	// Start the informers
	dynamicInformerFactory.Start(ctx.Done())

	// Wait for the cache to be synced.
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	select {
	case <-ctx.Done():
		// Get the final status
		proxy, err := httpProxyInformer.Lister().ByNamespace(obj.GetNamespace()).Get(obj.GetName())

		if err != nil {
			return fmt.Errorf("proxy deployment timed out, name: %s, namespace %s, error occurred while fetching latest status: %w", obj.GetName(), obj.GetNamespace(), err)
		}

		p := contourv1.HTTPProxy{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(proxy.(*unstructured.Unstructured).Object, &p)
		if err != nil {
			return fmt.Errorf("proxy deployment timed out, name: %s, namespace %s, error occurred while fetching latest status: %w", obj.GetName(), obj.GetNamespace(), err)
		}

		status := contourv1.DetailedCondition{}
		if len(p.Status.Conditions) > 0 {
			status = p.Status.Conditions[len(p.Status.Conditions)-1]
		}
		return fmt.Errorf("HTTP proxy deployment timed out, name: %s, namespace %s, status: %s, reason: %s", obj.GetName(), obj.GetNamespace(), status.Message, status.Reason)
	case err := <-doneCh:
		if err == nil {
			logger.Info(fmt.Sprintf("Marking HTTP proxy deployment %s in namespace %s as complete", obj.GetName(), obj.GetNamespace()))
		}
		return err
	}
}

func (handler *httpProxyWaiter) checkHTTPProxyStatus(ctx context.Context, dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory, obj client.Object, doneCh chan<- error) bool {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("httpProxyName", obj.GetName(), "namespace", obj.GetNamespace())
	proxy, err := dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR).Lister().ByNamespace(obj.GetNamespace()).Get(obj.GetName())
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to get http proxy: %s", err.Error()))
		return false
	}

	p := contourv1.HTTPProxy{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(proxy.(*unstructured.Unstructured).Object, &p)
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to convert http proxy: %s", err.Error()))
		return false
	}
	p.SetGroupVersionKind(contourv1.SchemeGroupVersion.WithKind("HTTPProxy"))

	if isContourHTTPProxyRouteChild(&p) {
		// This is a route HTTP proxy. We will not validate deployment completion for it and return success here
		logger.Info(fmt.Sprintf("Not validating the deployment of route HTTP proxy for %s", p.Name))
		doneCh <- nil
		return true
	}

	// We will check the status for the root HTTP proxy
	if p.Status.CurrentStatus == HTTPProxyStatusInvalid {
		if strings.Contains(p.Status.Description, "see Errors for details") {
			var msg strings.Builder
			for _, c := range p.Status.Conditions {
				if c.ObservedGeneration != p.Generation {
					continue
				}
				if c.Type == HTTPProxyConditionValid && c.Status == "False" {
					for _, e := range c.Errors {
						msg.WriteString(fmt.Sprintf("Error - Type: %s, Status: %s, Reason: %s, Message: %s\n", e.Type, e.Status, e.Reason, e.Message))
					}
				}
			}
			doneCh <- errors.New(msg.String())
		} else {
			doneCh <- fmt.Errorf("Failed to deploy HTTP proxy. Description: %s", p.Status.Description)
		}
		return false
	} else if p.Status.CurrentStatus == HTTPProxyStatusValid {
		// The HTTPProxy is ready
		doneCh <- nil
		return true
	}
	return false
}

func isContourHTTPProxy(obj client.Object) bool {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return gvk.Group == contourv1.SchemeGroupVersion.Group && strings.EqualFold(gvk.Kind, "HTTPProxy")
}

func isContourHTTPProxyRouteChild(obj client.Object) bool {
	if !isContourHTTPProxy(obj) {
		return false
	}

	var proxy contourv1.HTTPProxy
	switch p := obj.(type) {
	case *contourv1.HTTPProxy:
		proxy = *p
	case *unstructured.Unstructured:
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.Object, &proxy)
		if err != nil {
			return false
		}
	default:
		return false
	}

	return proxy.Spec.VirtualHost == nil && len(proxy.Spec.Includes) == 0 && len(proxy.Spec.Routes) > 0
}
