# Scaling

When handling large amounts of readers or publishers, streaming performance might get degraded due to bottlenecks in the underlying hardware infrastructure. In case of streaming without re-encoding (which is what MediaMTX does), these bottlenecks are almost always related to the limited bandwidth between server and readers. This issue can be strongly mitigated by enrolling horizontal scaling, which means deploying multiple coordinated server instances, and evenly distributing load on them.

There are several methods available to implement horizontal scaling, the main ones are described in this page.

## Read replicas

The main technique for handling additional readers consists in instantiating additional MediaMTX instances, called read replicas, that are in charge of picking streams from a MediaMTX "origin" instance and serving them to users. User connections and requests are distributed to each replica by a load balancer.

![read replicas](read-replicas.svg)

Publishers are meant to publish to the origin instance.

Read replicas must be configured in this way:

- Add one or more proxy paths, pointing to the MediaMTX origin instance, as described in [Proxy](12-proxy.md).
- If the protocol used by readers is WebRTC, disable `webrtcLocalUDPAddress` and `webrtcLocalTCPAddress` and enable a STUN server, as described in [Solving WebRTC connectivity issues](26-webrtc-specific-features.md#solving-webrtc-connectivity-issues).

The load balancer has to behave differently depending on the protocol(s) readers are gonna use to read the stream:

- When using RTSP, RTMP, SRT, the load balancer must be a Layer 4 LB.
- When using HLS and WebRTC, the load balancer must be a Layer 7 LB with sticky sessions enabled. Sticky sessions are needed to forward HTTP requests from the same user to the same replica, since a single HLS or WebRTC session is composed of several HTTP requests.

### Generic implementation

1. On the machine meant to host the MediaMTX origin instance, launch the instance:

   ```sh
   docker run -d \
   --name mediamtx \
   --restart always \
   --network host \
   bluenviron/mediamtx:1
   ```

2. On machines meant to host read replicas, create this MediaMTX configuration:

   ```yml
   webrtcLocalUDPAddress:
   webrtcICEServers2:
     - url: stun:stun.l.google.com:19302

   paths:
     "~^(.+)$":
       source: rtsp://dns-of-origin:8554/$G1
       sourceOnDemand: yes
   ```

   Replace `dns-of-origin` with the DNS or IP of the origin machine.

   Then launch MediaMTX:

   ```sh
   docker run -d \
   --name mediamtx \
   --restart always \
   --network host \
   -v $PWD/mediamtx.yml:/mediamtx.yml \
   bluenviron/mediamtx:1
   ```

3. On machines meant to host the load balancer, create this [Traefik](https://traefik.io/) configuration (`traefik.yml`):

   ```yml
   entryPoints:
     rtsp:
       address: ":8554"
     rtmp:
       address: ":1935"
     srt:
       address: ":8890/udp" # SRT usually uses UDP
     hls:
       address: ":8888"
     webrtc:
       address: ":8889"

   providers:
     file:
       filename: "/etc/traefik/dynamic_conf.yml"
       watch: true
   ```

   Create this second, dynamic, configuration (`dynamic_conf.yml`):

   ```yml
   tcp:
     routers:
       rtsp-router:
         rule: "HostSNI(`*`)"
         entryPoints: ["rtsp"]
         service: "rtsp-service"
       rtmp-router:
         rule: "HostSNI(`*`)"
         entryPoints: ["rtmp"]
         service: "rtmp-service"

     services:
       rtsp-service:
         loadBalancer:
         servers:
           - address: "replica-1-ip:8554"
           - address: "replica-2-ip:8554"
       rtmp-service:
         loadBalancer:
         servers:
           - address: "replica-1-ip:1935"
           - address: "replica-2-ip:1935"

   udp:
     routers:
       srt-router:
         entryPoints: ["srt"]
         service: "srt-service"
     services:
       srt-service:
         loadBalancer:
         servers:
           - address: "replica-1-ip:8890"
           - address: "replica-2-ip:8890"

   http:
     routers:
       hls-router:
         rule: "PathPrefix(`/`)"
         entryPoints: ["hls"]
         service: "hls-service"
       webrtc-router:
         rule: "PathPrefix(`/`)"
         entryPoints: ["webrtc"]
         service: "webrtc-service"

     services:
       hls-service:
         loadBalancer:
           sticky:
             cookie:
               name: "SERVERID"
         servers:
           - url: "http://replica-1-ip:8888"
           - url: "http://replica-2-ip:8888"
       webrtc-service:
         loadBalancer:
           sticky:
             cookie:
               name: "SERVERID"
         servers:
           - url: "http://replica-1-ip:8889"
           - url: "http://replica-2-ip:8889"
   ```

   Then launch Traefik:

   ```
   docker run -d \
   --name traefik \
   --restart always \
   --network host \
   -v $PWD/traefik.yml:/etc/traefik/traefik.yml \
   -v $PWD/dynamic_conf.yml:/etc/traefik/dynamic_conf.yml \
   traefik:v3.6.14
   ```

You can now use the IP address or DNS of the load balancer machines to read streams with any protocol.

### AWS-based implementation

1. Create a _Security group_ called `mediamtx-load-balancer`, that will be used by the load balancers. In _Inbound rules_, add:
   - a rule with type _All TCP_, source `0.0.0.0/0` (anywhere).
   - a rule with type _All UDP_, source `0.0.0.0/0` (anywhere).

2. Create a _Security group_ called `mediamtx-read-replicas`, that will be used by the read replicas. In _Inbound rules_, add:
   - a rule with type _All TCP_, source _Custom_, pick the `mediamtx-load-balancer` security group.
   - a rule with type _All UDP_, source _Custom_, pick the `mediamtx-load-balancer` security group.

3. Create a _Security group_ called `mediamtx-origin`, that will be used by the origin. In _Inbound rules_, add:
   - a rule with type _Custom TCP_, port `8554`, source _Custom_, pick the `mediamtx-read-replicas` security group.
   - a rule with type _All UDP_, source _Custom_, pick the `mediamtx-read-replicas` security group.
   - a rule of type _Custom TCP_, port `8554`. In the _source_ field, insert the IP range of publishers.
   - a rule of type _All UDP_. In the _Source_ field, insert the IP range of publishers.

4. Launch an EC2 instance that is meant to host the MediaMTX origin instance. Pick the _Amazon Linux_ AMI. Associate the `mediamtx-origin` _Security group_ to the instance. In _Advanced Details_, in the _User data_ textarea, copy and paste this:

   ```sh
   #!/bin/bash
   dnf update -y
   dnf install -y docker
   systemctl start docker
   systemctl enable docker
   usermod -aG docker ec2-user

   docker run -d \
   --name mediamtx \
   --restart always \
   --network host \
   bluenviron/mediamtx:1
   ```

5. Create a _Launch template_ for the read replicas. Pick the _Amazon Linux_ AMI. Associate the `mediamtx-read-replicas` _Security group_ to the launch template. In _Advanced Details_, in the _User data_ textarea, copy and paste this:

   ```sh
   #!/bin/bash
   dnf update -y
   dnf install -y docker
   systemctl start docker
   systemctl enable docker
   usermod -aG docker ec2-user

   mkdir -p /etc/mediamtx/
   tee /etc/mediamtx/mediamtx.yml << EOF
   webrtcLocalUDPAddress:
   webrtcICEServers2:
     - url: stun:stun.l.google.com:19302

   paths:
     "~^(.+)$":
       source: rtsp://dns-of-origin:8554/\$G1
       sourceOnDemand: yes
   EOF

   docker run -d \
   --name mediamtx \
   --restart always \
   --network host \
   -v /etc/mediamtx/mediamtx.yml:/mediamtx.yml \
   bluenviron/mediamtx:1
   ```

   Replace `dns-of-origin` with the private DNS of the origin EC2 instance.

6. Create several _Target groups_, with target type _Instances_, one for each of the following Protocol / Port combinations:
   - Name `mediamtx-read-replicas-8554`, Protocol _TCP_, port `8554` (RTSP), health check protocol _TCP_.
   - Name `mediamtx-read-replicas-1935`, Protocol _TCP_, port `1935` (RTMP), health check protocol _TCP_.
   - Name `mediamtx-read-replicas-8890`, Protocol _UDP_, port `8890` (SRT), health check protocol _TCP_, Advanced health check settings, Health check port, override, insert `8554`.
   - Name `mediamtx-read-replicas-8888`, Protocol _HTTP_, port `8888` (HLS), health check protocol _HTTP_, Advanced health check settings, Health check success codes `404`.
   - Name `mediamtx-read-replicas-8889`, Protocol _HTTP_, port `8889` (WebRTC), health check protocol _HTTP_, Advanced health check settings, Health check success codes `404`.

   Do not associate these target groups with any instance for now.

7. Select the `mediamtx-read-replicas-8888` target group, tab _Attributes_, _Edit_, section _Target selection configuration_, _Turn on stickiness_, _Save_. Do the same for the `mediamtx-read-replicas-8889` target group.

8. Create a _Load Balancer_, type _Network Load Balancer_. Associate the `mediamtx-load-balancer` _Security group_ with the load balancer. In _Listeners_, define 3 listeners:
   - Protocol _TCP_, port `8554` (RTSP), forward to target group `mediamtx-read-replicas-8554`
   - Protocol _TCP_, port `1935` (RTMP), forward to target group `mediamtx-read-replicas-1935`
   - Protocol _UDP_, port `8890` (SRT), forward to target group `mediamtx-read-replicas-8890`

9. Create a _Load Balancer_, type _Application Load Balancer_. Associate the `mediamtx-load-balancer` _Security group_ with the load balancer. In _Listeners_, define 2 listeners:
   - Protocol _HTTP_, port `8888` (HLS), forward to target group `mediamtx-read-replicas-8888`
   - Protocol _HTTP_, port `8889` (WebRTC), forward to target group `mediamtx-read-replicas-8889`

10. Create an _Auto scaling group_ for the read replicas. Pick the launch template that was defined before. CPU and Memory parameters are usually not important. In the _Load balancing_ section, pick _Attach to an existing load balancer_ and select all 5 target groups that were created before. In the _Group size_ section, in _Desired capacity_, set the desired instance count.

You can now use the DNS of the _Network Load Balancer_ to read streams with RTSP, RTMP and SRT, and the DNS of the _Application Load Balancer_ to read streams with HLS and WebRTC.

This process involved all available protocols, but it can be greatly simplified if users are meant to read streams with a single protocol only (for instance, WebRTC), in this case, opening specific ports only (8889) and creating specific load balancers only (Application load balancer) is enough.
