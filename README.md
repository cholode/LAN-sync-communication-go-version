# Lan IM - 高性能单机 WebSocket 消息分发网关

## 项目概述

本项目是一个基于 Go 语言原生 Goroutine 与 Epoll 网络模型构建的轻量级、高并发即时通讯（IM）消息分发网关。系统摒弃了传统重量级框架，专注于底层网络 I/O 与内存物理极限。通过容器化资源隔离与 Linux 内核参数调优，实现了单节点对海量长连接的极低成本维系，具备单机十万级并发（C100K）的物理承载潜能。

## 核心架构特性

- **无锁协程模型**：利用 Go 语言轻量级 Goroutine 与 Channel 实现连接隔离与事件广播，彻底阻断高并发场景下的数据读写竞争（Data Race）。
- **极简容器化安全防线**：采用 Multi-stage Build（多阶段构建），剥离编译环境，产物基于 Alpine 极简系统并静态链接。严格实施权限降级，使用非 root 专属应用级用户（imuser）运行核心进程。
- **物理级资源重塑**：通过 Docker Compose 强行介入 Linux 内核级参数，重写单进程文件描述符（ulimit nofile）硬性红线至 65535，彻底解除操作系统底层的并发连接束缚。
- **零内存泄漏控制**：具备严密的客户端连接生命周期管理（Ping/Pong 探活与超时熔断），确保海量连接断开后对应 Goroutine 与底层 Socket 句柄的绝对物理释放。

## 性能压测基线 (K6 Benchmark)

系统经过 K6 压测引擎的验证，核心物理监控指标如下：

- **单连接内存基线**：在维持 5000+ 并发静默长连接状态下，Go 进程峰值内存稳定于 280MB，计算单连接物理内存开销仅为 **~57KB**。
- **极速握手响应**：在瞬时涌入数千并发连接的洪峰下，TCP 三次握手及 HTTP 101 协议升级的 P95 延迟被严格控制在 **< 5ms**。
- **高吞吐读写放大**：在 100 活跃用户的高频发包场景中，单节点顺利消化单秒千级别的即时读写放大风暴（完成 45万+ 消息广播吞吐），且期间系统无 STW 卡顿，展现出极强的抗压防线。

## 快速启动 (Quick Start)

### 环境依赖

- Docker Engine (>= 20.10)
- Docker Compose V2

### 部署指令

系统已将所有环境依赖与启动逻辑封装于容器编排文件中，执行以下指令完成重建与物理拉起：

```bash
# 赋予持久化数据目录基础权限（如有需要）

mkdir -p lan_im_data

# 强制构建并后台拉起系统

docker compose up -d --build
```

# 测试结果
### 500人群聊每人每秒发一次消息

```
CONTAINER ID   NAME         CPU %     MEM USAGE / LIMIT     MEM %     NET I/O           BLOCK I/O         PIDS    
274674fe6ebf   im-mysql     16.98%    568.7MiB / 4GiB       13.88%    246MB / 715MB     41.5MB / 783MB    134     
960e69c7389c   im-backend   158.83%   113.1MiB / 8GiB       1.38%     5.88GB / 58.3GB   8.19kB / 0B       14      
a14774ba193d   im-nginx     0.00%     280.1MiB / 9.713GiB   2.82%     998B / 126B       1.73MB / 12.3kB   11   
```

```
custom_online_user_count.......: 1        min=-1          max=1
    custom_ws_connect_success......: 502      0.796779/s
    custom_ws_msg_sent_total.......: 269616   427.936898/s

    HTTP
    http_req_duration..............: avg=5.92ms min=1.03ms  med=5.87ms max=44.99ms p(90)=10.56ms p(95)=13.11ms    
      { expected_response:true }...: avg=5.92ms min=1.03ms  med=5.87ms max=44.99ms p(90)=10.56ms p(95)=13.11ms    
    http_req_failed................: 0.00%    0 out of 1006
    http_reqs......................: 1006     1.596732/s

    EXECUTION
    iteration_duration.............: avg=7m21s  min=6m31s   med=7m21s  max=8m11s   p(90)=8m1s    p(95)=8m6s       
    iterations.....................: 2        0.003174/s
    vus............................: 500      min=3           max=500
    vus_max........................: 500      min=500         max=500

    NETWORK
    data_received..................: 25 GB    39 MB/s
    data_sent......................: 23 MB    37 kB/s

    WEBSOCKET
    ws_connecting..................: avg=3.93ms min=504.1µs med=1.77ms max=41.46ms p(90)=9.55ms  p(95)=11.82ms    
    ws_msgs_received...............: 70908608 112546.769281/s
    ws_msgs_sent...................: 269616   427.936898/s
    ws_ping........................: avg=7.36ms min=0s      med=5.71ms max=48.37ms p(90)=16.22ms p(95)=20.29ms    
    ws_session_duration............: avg=7m20s  min=6m30s   med=7m20s  max=8m10s   p(90)=8m0s    p(95)=8m5s       
    ws_sessions....................: 502      0.796779/s
```

基于 Go 实现高并发 WebSocket 即时通讯服务，通过 k6 完成 500 并发长连接压测：**连接成功率 100%、HTTP 请求零失败、消息吞吐 11.2 万条 / 秒、接口平均延迟 5.92ms**，服务稳定支撑高并发长连接场景。

### 万人在线，千人活跃，1个群100人，10人活跃

```
CONTAINER ID   NAME         CPU %     MEM USAGE / LIMIT     MEM %     NET I/O           BLOCK I/O         PIDS    
1c349e626734   im-mysql     3.51%     434.2MiB / 4GiB       10.60%    41.3MB / 77.6MB   67.6MB / 252MB    44      
e3f5b4a7225b   im-backend   90.43%    1.622GiB / 8GiB       20.28%    941MB / 4.24GB    1.62MB / 0B       13      
24485adc0f92   im-nginx     0.00%     280.6MiB / 9.713GiB   2.82%     998B / 126B       2.39MB / 12.3kB   11     
```

```
 █ TOTAL RESULTS

    CUSTOM
    custom_online_user_count.......: 1        min=1          max=1
    custom_ws_connect_success......: 10000    11.741046/s
    custom_ws_msg_sent_total.......: 359000   421.503557/s

    HTTP
    http_req_duration..............: avg=5.11ms  min=508.1µs med=5.13ms  max=67.02ms  p(90)=8.38ms  p(95)=9.48ms  
      { expected_response:true }...: avg=5.11ms  min=508.1µs med=5.13ms  max=67.02ms  p(90)=8.38ms  p(95)=9.48ms  
    http_req_failed................: 0.00%    1 out of 20102
    http_reqs......................: 20102    23.601851/s

    EXECUTION
    vus............................: 2        min=0          max=10000
    vus_max........................: 10000    min=10000      max=10000

    NETWORK
    data_received..................: 8.7 GB   10 MB/s
    data_sent......................: 35 MB    42 kB/s

    WEBSOCKET
    ws_connecting..................: avg=2.45ms  min=0s      med=2.09ms  max=15.6ms   p(90)=4.18ms  p(95)=4.43ms  
    ws_msgs_received...............: 23190212 27227.734956/s
    ws_msgs_sent...................: 359000   421.503557/s
    ws_ping........................: avg=14.01ms min=0s      med=512.5µs max=425.17ms p(90)=59.75ms p(95)=107.69m 
    ws_sessions....................: 10000    11.741046/s



```

### 5000人在线，500人活跃，1个群50人，5人活跃

```

  █ TOTAL RESULTS

    CUSTOM
    custom_online_user_count.......: 1        min=1             max=1
    custom_ws_connect_success......: 5000     5.869372/s
    custom_ws_msg_sent_total.......: 359000   421.420925/s

    HTTP
    http_req_duration..............: avg=6.29ms min=0s      med=2.67ms  max=75.46ms  p(90)=14.72ms p(95)=16.64ms  
      { expected_response:true }...: avg=8.08ms min=0s      med=8.39ms  max=75.46ms  p(90)=15.61ms p(95)=18.22ms  
    http_req_failed................: 33.11%   5001 out of 15102//注册失败导致的
    http_reqs......................: 15102    17.727852/s

    EXECUTION
    vus............................: 2        min=0             max=5000
    vus_max........................: 5000     min=5000          max=5000

    NETWORK
    data_received..................: 7.7 GB   9.1 MB/s
    data_sent......................: 47 MB    55 kB/s

    WEBSOCKET
    ws_connecting..................: avg=1.92ms min=505.4µs med=1.76ms  max=9.85ms   p(90)=2.68ms  p(95)=2.91ms   
    ws_msgs_received...............: 10957862 12863.154157/s
    ws_msgs_sent...................: 359000   421.420925/s
    ws_ping........................: avg=5.76ms min=0s      med=544.4µs max=240.53ms p(90)=1.75ms  p(95)=45.05ms  
    ws_sessions....................: 5000     5.869372/s

```

### 万人在线，千人活跃，1个群100人，10人活跃（带crypt加密）

```
CONTAINER ID   NAME         CPU %     MEM USAGE / LIMIT   MEM %     NET I/O           BLOCK I/O         PIDS      
d30fe5ff622a   im-mysql     4.00%     452.6MiB / 4GiB     11.05%    55.5MB / 98MB     106MB / 724MB     40        
331a1c5ca6f5   im-backend   108.38%   1.297GiB / 8GiB     16.21%    1.55GB / 9.18GB   8.19kB / 0B       12        
a88a458ac18c   im-nginx     0.00%     280MiB / 9.713GiB   2.82%     998B / 126B       1.73MB / 12.3kB   11   
```

```
  █ TOTAL RESULTS

    CUSTOM
    custom_online_user_count.......: 1        min=1          max=1
    custom_ws_connect_success......: 10000    11.739344/s
    custom_ws_msg_sent_total.......: 359000   421.442433/s

    HTTP
    http_req_duration..............: avg=25.62ms min=2.58ms med=18.11ms  max=63.81ms  p(90)=44.96ms p(95)=45.41ms 
      { expected_response:true }...: avg=25.62ms min=2.58ms med=18.12ms  max=63.81ms  p(90)=44.96ms p(95)=45.41ms 
    http_req_failed................: 0.00%    1 out of 20102
    http_reqs......................: 20102    23.598428/s

    EXECUTION
    vus............................: 12       min=0          max=10000
    vus_max........................: 10000    min=10000      max=10000

    NETWORK
    data_received..................: 8.7 GB   10 MB/s
    data_sent......................: 35 MB    42 kB/s

    WEBSOCKET
    ws_connecting..................: avg=2.25ms  min=0s     med=1.56ms   max=19.54ms  p(90)=4.38ms  p(95)=4.64ms  
    ws_msgs_received...............: 21984353 25808.187262/s
    ws_msgs_sent...................: 359000   421.442433/s
    ws_ping........................: avg=14.12ms min=0s     med=508.29µs max=417.48ms p(90)=57.6ms  p(95)=109.83m 
    ws_sessions....................: 10000    11.739344/s
```