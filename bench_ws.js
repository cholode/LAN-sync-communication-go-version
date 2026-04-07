import ws from 'k6/ws';
import http from 'k6/http';
import exec from 'k6/execution';
import { sleep } from 'k6';
import { Counter, Gauge } from 'k6/metrics';

// ============================================================================
// 指标采集器定义 (严格隔离 K6 底层保留字)
// ============================================================================
const wsMsgSentTotal = new Counter('custom_ws_msg_sent_total');
const wsConnectSuccess = new Counter('custom_ws_connect_success');
const httpReqFailed = new Counter('custom_http_req_failed');
const onlineUserGauge = new Gauge('custom_online_user_count');

// ============================================================================
// 压测沙盘全局拓扑配置
// ============================================================================
const TOTAL_VUS = 10000; // 全局物理并发连接总数
const RAMP_UP_DURATION = '2m'; // 【架构调优】节点爬坡阶段：拉长至 2 分钟平滑建立连接
const FIRE_DURATION = '5m'; // 极值压测阶段：持续 5 分钟火力输出
const RAMP_DOWN_DURATION = '1m'; // 平滑降级阶段：最后 1 分钟陆续断开
const TOTAL_ROOMS = 100; // 业务沙盘：测试群组总数
const ACTIVE_PER_ROOM = 10; // 业务沙盘：每个群组内的火力输出节点数
const BASE_URL = 'http://127.0.0.1:8080/api/v1';
const WS_URL = 'ws://127.0.0.1:8080/api/v1/ws';

// ============================================================================
// 引擎调度器配置
// ============================================================================
export const options = {
  scenarios: {
    im_5000_pressure_test: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: RAMP_UP_DURATION, target: TOTAL_VUS }, // 0~2分钟：平滑建连，减轻网关握手压力
        { duration: FIRE_DURATION, target: TOTAL_VUS },    // 2~7分钟：保持满载并发，检验系统极值
        { duration: RAMP_DOWN_DURATION, target: 0 },       // 7~8分钟：物理连接销毁，释放 FD
      ],
      gracefulRampDown: '10s', // 允许进行中请求拥有 10 秒的清理缓冲期
    },
  },
  // 强制丢弃 HTTP 响应体，避免 V8 引擎产生 GC 停顿引发压测端假死
  discardResponseBodies: true,
};

// ============================================================================
// 生命周期阶段 1: 全局基础设施构建 (Setup)
// ============================================================================
export function setup() {
  console.log("[架构基建] 开始初始化压测沙盘拓扑结构...");
  const headers = { 'Content-Type': 'application/json' };
  const adminAccount = { username: "admin_test_master", password: "password123" };

  // 执行管理员认证与 Token 颁发
  http.post(`${BASE_URL}/register`, JSON.stringify(adminAccount), { headers });
  const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify(adminAccount), {
    headers,
    responseType: 'text' // 局部豁免：强制读取响应体以提取 JWT
  });

  if (loginRes.status !== 200) {
    console.error("[致命异常] 超级管理员认证失败，系统阻断压测初始化流程");
    return { roomIds: [1] }; // 物理兜底，防止后续数组越界
  }

  const adminToken = JSON.parse(loginRes.body).token;
  let roomIds = [];

  // 批量构建业务群组拓扑
  for (let i = 0; i < TOTAL_ROOMS; i++) {
    const roomRes = http.post(`${BASE_URL}/rooms`, JSON.stringify({ name: `压测沙盘群组_${i+1}` }), {
      headers: {
        'Authorization': `Bearer ${adminToken}`,
        'Content-Type': 'application/json'
      },
      responseType: 'text'
    });

    if (roomRes.status === 200) {
      const parsed = JSON.parse(roomRes.body);
      const id = parsed.data?.id || parsed.id || (i+1);
      roomIds.push(id);
    } else {
      roomIds.push(i+1);
    }
  }

  console.log(`[架构基建] 成功构建 ${roomIds.length} 个隔离群组，沙盘就绪。`);
  return { roomIds };
}

// ============================================================================
// 生命周期阶段 2: 并发节点执行核心 (VU Runtime)
// ============================================================================
export default function (data) {
  const vuId = exec.vu.idInTest;

  // 路由分配与角色映射
  const roomIndex = Math.floor((vuId - 1) / (TOTAL_VUS / TOTAL_ROOMS));
  const roomId = data.roomIds[roomIndex] || 1;

  // 依据模运算精准控制活跃节点比例
  const isActiveUser = (vuId % (TOTAL_VUS / TOTAL_ROOMS) >= 1 && vuId % (TOTAL_VUS / TOTAL_ROOMS) <= ACTIVE_PER_ROOM);

  // 必须使用特定前缀触发后端哈希旁路机制
  const userAccount = { username: `silent_vu_${vuId}`, password: "password123" };
  const headers = { 
      'Content-Type': 'application/json',
      'Connection': 'close'
  };

  // ------------------------------------------------------------------------
  // 协议握手层: HTTP 认证与路由加入
  // ------------------------------------------------------------------------
  const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify(userAccount), {
    headers,
    responseType: 'text'
  });
  if (loginRes.status !== 200) {
    httpReqFailed.add(1);
    sleep(60);
    return;
  }

  const token = JSON.parse(loginRes.body).token;
  const joinRes = http.post(`${BASE_URL}/rooms/${roomId}/join`, null, {
    headers: { Authorization: `Bearer ${token}`,'Connection': 'close' }
  });
  if (joinRes.status !== 200 && joinRes.status !== 400) {
    httpReqFailed.add(1);
    sleep(60);
    return;
  }

  // ------------------------------------------------------------------------
  // 物理连接层: WebSocket 全双工通道维护
  // ------------------------------------------------------------------------
  const wsConnectUrl = `${WS_URL}?token=${token}&room_id=${roomId}`;
  ws.connect(wsConnectUrl, null, (socket) => {
    let msgTimer;

    socket.on('open', () => {
      wsConnectSuccess.add(1);
      onlineUserGauge.add(1);

      // TCP 保活机制: K6 引擎将在 socket 关闭时自动销毁该定时器
      socket.setInterval(() => socket.ping(), 10000);

      // 活跃节点火力调度机制
      if (isActiveUser) {
        const elapsedMs = new Date().getTime() - exec.scenario.startTime;
        
        // 【核心物理修正】爬坡期绝对时间阈值：2m = 120000ms
        const fireStartWaitMs = 120000 - elapsedMs; 
        
        // 【核心物理修正】停止开火的绝对时间阈值：爬坡 2m + 开火 5m = 7m (420000ms)
        const stopFiringMs = 420000; 

        const startFiring = () => {
          msgTimer = socket.setInterval(() => {
            const currentElapsedMs = new Date().getTime() - exec.scenario.startTime;
            
            // 触发火力阻断机制：一旦触碰 7 分钟时间线，立刻停止发送并准备撤离
            if (currentElapsedMs >= stopFiringMs) {
              clearInterval(msgTimer); // 标准的全局定时器回收调用
              return;
            }

            // 载荷组装与物理下发
            const msg = JSON.stringify({
              room_id: roomId,
              type: 1,
              content: ` VU_ID:[${vuId}]}]`
            });
            socket.send(msg);
            wsMsgSentTotal.add(1);
          }, 1000);
        };

        // 基于全局时钟进行绝对时间对齐同步
        if (fireStartWaitMs > 0) {
          socket.setTimeout(startFiring, fireStartWaitMs);
        } else {
          startFiring();
        }
      }
    });

    socket.on('close', () => {
      onlineUserGauge.add(-1);
      // 依赖 K6 原生资源回收机制，不再手动执行冗余的 clearInterval
    });

    socket.on('error', (e) => {
      const errMsg = e.error();
      if (errMsg !== "websocket: close 1006 (abnormal closure)" && errMsg !== "normal closure") {
        console.error(`[通道异常] 虚拟用户 ${vuId} 连接阻断: ${errMsg}`);
      }
    });
  });

  sleep(1);
}

// ============================================================================
// 生命周期阶段 3: 物理资源回收 (Teardown)
// ============================================================================
export function teardown(data) {
  console.log(`[战役结束] 压测节点已全量撤离，物理沙盘销毁完毕。`);
}