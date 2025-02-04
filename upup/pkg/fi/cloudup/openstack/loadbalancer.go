/*
Copyright 2019 The Kubernetes Authors.

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

package openstack

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	v2pools "github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kops/util/pkg/vfs"
)

// memberBackoff is the backoff strategy for openstack updating members in loadbalancer pool
var memberBackoff = wait.Backoff{
	Duration: time.Second,
	Factor:   2,
	Jitter:   0.1,
	Steps:    10,
}

func (c *openstackCloud) CreatePoolMonitor(opts monitors.CreateOpts) (*monitors.Monitor, error) {
	return createPoolMonitor(c, opts)
}

func createPoolMonitor(c OpenstackCloud, opts monitors.CreateOpts) (poolMonitor *monitors.Monitor, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		poolMonitor, err = monitors.Create(context.TODO(), c.LoadBalancerClient(), opts).Extract()
		if err != nil {
			return false, fmt.Errorf("failed to create pool monitor: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return poolMonitor, err
	}
	return poolMonitor, nil
}

func (c *openstackCloud) ListMonitors(opts monitors.ListOpts) (monitorList []monitors.Monitor, err error) {
	return listMonitors(c, opts)
}

func listMonitors(c OpenstackCloud, opts monitors.ListOpts) (monitorList []monitors.Monitor, err error) {
	if c.LoadBalancerClient() == nil {
		return monitorList, fmt.Errorf("loadbalancer support not available in this deployment")
	}
	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		allPages, err := monitors.List(c.LoadBalancerClient(), opts).AllPages(context.TODO())
		if err != nil {
			return false, fmt.Errorf("failed to list monitors: %s", err)
		}
		monitorList, err = monitors.ExtractMonitors(allPages)
		if err != nil {
			return false, fmt.Errorf("failed to extract monitor pages: %s", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return monitorList, err
	}
	return monitorList, nil
}

func (c *openstackCloud) DeleteMonitor(monitorID string) error {
	return deleteMonitor(c, monitorID)
}

func deleteMonitor(c OpenstackCloud, monitorID string) error {
	if c.LoadBalancerClient() == nil {
		return fmt.Errorf("loadbalancer support not available in this deployment")
	}
	done, err := vfs.RetryWithBackoff(deleteBackoff, func() (bool, error) {
		err := monitors.Delete(context.TODO(), c.LoadBalancerClient(), monitorID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting pool: %v", err)
		}
		if isNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeletePool(poolID string) error {
	return deletePool(c, poolID)
}

func deletePool(c OpenstackCloud, poolID string) error {
	if c.LoadBalancerClient() == nil {
		return fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(deleteBackoff, func() (bool, error) {
		err := v2pools.Delete(context.TODO(), c.LoadBalancerClient(), poolID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting pool: %v", err)
		}
		if isNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeleteListener(listenerID string) error {
	return deleteListener(c, listenerID)
}

func deleteListener(c OpenstackCloud, listenerID string) error {
	if c.LoadBalancerClient() == nil {
		return fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(deleteBackoff, func() (bool, error) {
		err := listeners.Delete(context.TODO(), c.LoadBalancerClient(), listenerID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting listener: %v", err)
		}
		if isNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeleteLB(lbID string, opts loadbalancers.DeleteOpts) error {
	return deleteLB(c, lbID, opts)
}

func deleteLB(c OpenstackCloud, lbID string, opts loadbalancers.DeleteOpts) error {
	if c.LoadBalancerClient() == nil {
		return fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(deleteBackoff, func() (bool, error) {
		err := loadbalancers.Delete(context.TODO(), c.LoadBalancerClient(), lbID, opts).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting loadbalancer: %v", err)
		}
		if isNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) CreateLB(opt loadbalancers.CreateOptsBuilder) (*loadbalancers.LoadBalancer, error) {
	return createLB(c, opt)
}

func createLB(c OpenstackCloud, opt loadbalancers.CreateOptsBuilder) (*loadbalancers.LoadBalancer, error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	var i *loadbalancers.LoadBalancer
	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		v, err := loadbalancers.Create(context.TODO(), c.LoadBalancerClient(), opt).Extract()
		if err != nil {
			return false, fmt.Errorf("error creating loadbalancer: %v", err)
		}
		i = v
		return true, nil
	})
	if err != nil {
		return i, err
	} else if done {
		return i, nil
	} else {
		return i, wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) GetLB(loadbalancerID string) (lb *loadbalancers.LoadBalancer, err error) {
	return getLB(c, loadbalancerID)
}

func getLB(c OpenstackCloud, loadbalancerID string) (lb *loadbalancers.LoadBalancer, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		lb, err = loadbalancers.Get(context.TODO(), c.LoadBalancerClient(), loadbalancerID).Extract()
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return lb, err
	}
	return lb, nil
}

// ListLBs will list load balancers
func (c *openstackCloud) ListLBs(opt loadbalancers.ListOptsBuilder) (lbs []loadbalancers.LoadBalancer, err error) {
	return listLBs(c, opt)
}

func listLBs(c OpenstackCloud, opt loadbalancers.ListOptsBuilder) (lbs []loadbalancers.LoadBalancer, err error) {
	if c.LoadBalancerClient() == nil {
		// skip error because cluster delete will otherwise fail
		return lbs, nil
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		allPages, err := loadbalancers.List(c.LoadBalancerClient(), opt).AllPages(context.TODO())
		if err != nil {
			return false, fmt.Errorf("failed to list loadbalancers: %s", err)
		}
		lbs, err = loadbalancers.ExtractLoadBalancers(allPages)
		if err != nil {
			return false, fmt.Errorf("failed to extract loadbalancer pages: %s", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return lbs, err
	}
	return lbs, nil
}

func (c *openstackCloud) GetLBStats(loadbalancerID string) (stats *loadbalancers.Stats, err error) {
	return getLBStats(c, loadbalancerID)
}

func getLBStats(c OpenstackCloud, loadbalancerID string) (stats *loadbalancers.Stats, err error) {
	if c.LoadBalancerClient() == nil {
		return stats, nil
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		stats, err = loadbalancers.GetStats(context.TODO(), c.LoadBalancerClient(), loadbalancerID).Extract()
		if err != nil {
			return false, fmt.Errorf("Error getting load balancer stats %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return stats, err
	}
	return stats, nil
}

func (c *openstackCloud) GetPool(poolID string) (pool *v2pools.Pool, err error) {
	return getPool(c, poolID)
}

func getPool(c OpenstackCloud, poolID string) (pool *v2pools.Pool, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		pool, err = v2pools.Get(context.TODO(), c.LoadBalancerClient(), poolID).Extract()
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return pool, err
	}
	return pool, nil
}

func (c *openstackCloud) GetPoolMember(poolID string, memberID string) (member *v2pools.Member, err error) {
	return getPoolMember(c, poolID, memberID)
}

func getPoolMember(c OpenstackCloud, poolID string, memberID string) (member *v2pools.Member, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		member, err = v2pools.GetMember(context.TODO(), c.LoadBalancerClient(), poolID, memberID).Extract()
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return member, err
	}
	return member, nil
}

func (c *openstackCloud) UpdateMemberInPool(poolID string, memberID string, opts v2pools.UpdateMemberOptsBuilder) (association *v2pools.Member, err error) {
	return updateMemberInPool(c, poolID, memberID, opts)
}

func updateMemberInPool(c OpenstackCloud, poolID string, memberID string, opts v2pools.UpdateMemberOptsBuilder) (association *v2pools.Member, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(memberBackoff, func() (bool, error) {
		association, err = v2pools.UpdateMember(context.TODO(), c.LoadBalancerClient(), poolID, memberID, opts).Extract()
		if err != nil {
			// member not found anymore
			if isNotFound(err) {
				return true, nil
			}
			// pool is currently in immutable state, try to retry
			if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				klog.Infof("got error %v retrying...", http.StatusConflict)
				return false, nil
			}
			return false, fmt.Errorf("failed to update pool membership: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return association, err
	}
	return association, nil
}

func (c *openstackCloud) AssociateToPool(server *servers.Server, poolID string, opts v2pools.CreateMemberOpts) (association *v2pools.Member, err error) {
	return associateToPool(c, server, poolID, opts)
}

func associateToPool(c OpenstackCloud, server *servers.Server, poolID string, opts v2pools.CreateMemberOpts) (association *v2pools.Member, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		association, err = v2pools.GetMember(context.TODO(), c.LoadBalancerClient(), poolID, server.ID).Extract()
		if err != nil || association == nil {
			// Pool association does not exist.  Create it
			association, err = v2pools.CreateMember(context.TODO(), c.LoadBalancerClient(), poolID, opts).Extract()
			if err != nil {
				return false, fmt.Errorf("failed to create pool association: %v", err)
			}
			return true, nil
		}
		// NOOP
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return association, err
	}
	return association, nil
}

func (c *openstackCloud) CreatePool(opts v2pools.CreateOpts) (pool *v2pools.Pool, err error) {
	return createPool(c, opts)
}

func createPool(c OpenstackCloud, opts v2pools.CreateOpts) (pool *v2pools.Pool, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		pool, err = v2pools.Create(context.TODO(), c.LoadBalancerClient(), opts).Extract()
		if err != nil {
			return false, fmt.Errorf("failed to create pool: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return pool, err
	}
	return pool, nil
}

func (c *openstackCloud) ListPoolMembers(poolID string, opts v2pools.ListMembersOpts) (memberList []v2pools.Member, err error) {
	return listPoolMembers(c, poolID, opts)
}

func listPoolMembers(c OpenstackCloud, poolID string, opts v2pools.ListMembersOpts) (memberList []v2pools.Member, err error) {
	if c.LoadBalancerClient() == nil {
		return memberList, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		memberPage, err := v2pools.ListMembers(c.LoadBalancerClient(), poolID, opts).AllPages(context.TODO())
		if err != nil {
			return false, fmt.Errorf("failed to list members: %v", err)
		}
		memberList, err = v2pools.ExtractMembers(memberPage)
		if err != nil {
			return false, fmt.Errorf("failed to extract members: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return memberList, err
	}
	return memberList, nil
}

func (c *openstackCloud) ListPools(opts v2pools.ListOpts) (poolList []v2pools.Pool, err error) {
	return listPools(c, opts)
}

func listPools(c OpenstackCloud, opts v2pools.ListOpts) (poolList []v2pools.Pool, err error) {
	if c.LoadBalancerClient() == nil {
		return poolList, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		poolPage, err := v2pools.List(c.LoadBalancerClient(), opts).AllPages(context.TODO())
		if err != nil {
			return false, fmt.Errorf("failed to list pools: %v", err)
		}
		poolList, err = v2pools.ExtractPools(poolPage)
		if err != nil {
			return false, fmt.Errorf("failed to extract pools: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return poolList, err
	}
	return poolList, nil
}

func (c *openstackCloud) ListListeners(opts listeners.ListOpts) (listenerList []listeners.Listener, err error) {
	return listListeners(c, opts)
}

func listListeners(c OpenstackCloud, opts listeners.ListOpts) (listenerList []listeners.Listener, err error) {
	if c.LoadBalancerClient() == nil {
		return listenerList, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		listenerPage, err := listeners.List(c.LoadBalancerClient(), opts).AllPages(context.TODO())
		if err != nil {
			return false, fmt.Errorf("failed to list listeners: %v", err)
		}
		listenerList, err = listeners.ExtractListeners(listenerPage)
		if err != nil {
			return false, fmt.Errorf("failed to extract listeners: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return listenerList, err
	}
	return listenerList, nil
}

func (c *openstackCloud) CreateListener(opts listeners.CreateOpts) (listener *listeners.Listener, err error) {
	return createListener(c, opts)
}

func createListener(c OpenstackCloud, opts listeners.CreateOpts) (listener *listeners.Listener, err error) {
	if c.LoadBalancerClient() == nil {
		return nil, fmt.Errorf("loadbalancer support not available in this deployment")
	}

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		listener, err = listeners.Create(context.TODO(), c.LoadBalancerClient(), opts).Extract()
		if err != nil {
			return false, fmt.Errorf("unabled to create listener: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return listener, err
	}
	return listener, nil
}
