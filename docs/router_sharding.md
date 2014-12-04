- how is router configuration visualized from a user perspective
- how is router configuration visualized from an admin perspective
- how is a user notified of a route route binding and final dns
- how does a user request default dns name vs custom dns name
- router fronting with DNS, how are entries created

## Description

As an application administrator, I would like my routes to be configured with shards so they can
grow beyond a single active/active or active/passive setup.  I should be able to configure many
routers to allocate user requested routes to and be able to visualize the configuration.  

## Use Cases

The following use cases should be satisfied by this proposal:

1.  Configure routers as OpenShift resources and let the platform keep the specified configuration
    running
1.  Create a single, unsharded router
1.  Create multiple routers with shards corresponding to a resource label
1.  Allow any router to run in an HA configuration
1.  User requests default route for application
1.  User requests custom route for application
1.  Create DNS (or other front end entry points) for routers

## Existing Artifacts

1.  Routing: https://github.com/pweil-/origin/blob/master/docs/routing.md
1.  HA Routing: https://github.com/pweil-/origin/blob/master/docs/routing.md#running-ha-routers
1.  DNS Round Robin: https://github.com/pweil-/origin/blob/master/docs/routing.md#dns-round-robin

## Configuring Routers

Administering routers as a top level object allows administrators to use custom commands specific
to routers.  This provides a more use friendly mechanism of configuration and customizing routers.
However, this also introduces more code for an object that will likely be dealt with as a pod
anyway.  Routers should be a low touch configuration item that do not require many custom commands
for daily administration.

Pros:

- Configuration lives in etcd, just like any other resource
- Shards are configured via custom commands and `json` syntax
- Routers are known to OpenShift; the system ensures the proper configuration is running
- Custom administration syntax
- Deal with routers as infra
- The system knows about routers for route route binding and visualization with no extra effort

Cons: 

- More divergent from Kubernetes codebase initially, though we may be able to generalize parts of
  this approach to sharding to other resources and controllers which allow sharding

## DNS

OPEN QUESTION: do we intend on hosting DNS for the Online use case?

1. NO: users map their domain to resolve our router ip(s).  Must be done after route binding.  User is responsible for balancing requests between routers?
2. YES: users configure their domain to point to our nameservers for resolution.  Can be done before route binding (nameserver IPs are known).  Allows us to add
routers to shards and have them picked up by DNS RR.  For custom DNS we make a CNAME that points to the wildcard shard entry
3. BOTH: we are still dealing with DNS cache issues

In order to facilitate supplying external DNS for applications in the OpenShift system the router configuration will be 
modified with an indicator that the DNS name is user owned or system controlled.

     {
        "type": "route",
        ...
        "dnsType": "system|user",
     }

1.  System supplied DNS: this indicates that the user *DOES NOT* own the domain name and is requesting that OpenShift 
supply it.  The user provides a `Host` that is used as a prefix to the final DNS name which is determined based on the router 
route binding and takes the form of: `<namespace>-<Host>.<shard>.v3.rhcloud.com.

1.  User supplied DNS: this indicates that the user currently owns a domain name and will be able to configure their 
registrar to indicate that OpenShift's DNS servers will provide DNS look ups for the domain.  When a user controlled DNS 
entry is request no manipulation will be done to the `Host` field of the `route` configuration.


#### DNS Implementations

DNS plugins will be able to watch the `router` configuration to determine the correct zone files to set up with wildcard 
entries.  It will also be able to watch the `route` configuration to make entries for user supplied DNS requests that map 
to a shard.

Example: 
    
    shard1.zone:
    $ORIGIN shard1.v3.rhcloud.com.
    
    @       IN      SOA     . shard1.v3.rhcloud.com. (
                         2009092001         ; Serial
                             604800         ; Refresh
                              86400         ; Retry
                            1206900         ; Expire
                                300 )       ; Negative Cache TTL
            IN      NS      ns1.v3.rhcloud.com.
    ns1     IN      A       127.0.0.1
    *       IN      A       10.245.2.2      ; active/active DNS round robin
            IN      A       10.245.2.3      ; active/active DNS round robin
            
    shard2.zone:
    $ORIGIN shard2.v3.rhcloud.com.
    
    @       IN      SOA     . shard2.v3.rhcloud.com. (
                         2009092001         ; Serial
                             604800         ; Refresh
                              86400         ; Retry
                            1206900         ; Expire
                                300 )       ; Negative Cache TTL
            IN      NS      ns1.v3.rhcloud.com.
    ns1     IN      A       127.0.0.1
    *       IN      A       10.245.2.4      ; active/active DNS round robin
            IN      A       10.245.2.5      ; active/active DNS round robin 
                       
    user_supplied.zone:
    $ORIGIN example.com.
    
    @       IN      SOA     . example.com. (
                         2009092001         ; Serial
                             604800         ; Refresh
                              86400         ; Retry
                            1206900         ; Expire
                                300 )       ; Negative Cache TTL
            IN      NS      ns1.v3.rhcloud.com.
    ns1     IN      A       127.0.0.1
    www     IN      CNAME   shard1.v3.rhcloud.com ; points to shard                           





