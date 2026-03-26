package servicechains

import (
	"context"

	"k8s.io/client-go/dynamic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Client struct {
	dynamicClient *dynamic.DynamicClient
}

func NewClient(dynamicClient *dynamic.DynamicClient) (*Client, error) {
	return &Client{
		dynamicClient: dynamicClient,
	}, nil
}

func (c *Client) Create(obj *ServiceChain) error {
	obj.Kind = "ServiceChain"
	obj.APIVersion = Resource.GroupVersion().String()
	_, err := c.dynamicClient.
								Resource(Resource).
								Namespace(obj.Namespace).
								Create(context.TODO(), obj.ToUnstructured(), metav1.CreateOptions{})
	return err
}

func (c *Client) Delete(name, namespace string) error {
	err := c.dynamicClient.
							Resource(Resource).
							Namespace(namespace).
							Delete(context.TODO(), name, metav1.DeleteOptions{})
	return err
}