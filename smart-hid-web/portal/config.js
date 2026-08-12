// Smart HID Portal — runtime config (zero-build, ES module).
//
// API_BASE 默认同源 /api/v1 —— 当 Cloud 通过 web_root 托管本门户时同源，零 CORS。
// 本地开发如需指向别处的 Cloud，在浏览器控制台执行：
//   localStorage.setItem('smarthid_cloud_base', 'http://localhost:17880/api/v1')
// 然后刷新即可。

const override = localStorage.getItem('smarthid_cloud_base');
export const API_BASE = override || '/api/v1';

export const TOKEN_KEY = 'smarthid_token';
export const USER_KEY = 'smarthid_user';

// Device ID 必须匹配此正则（与 Cloud 后端 deviceIDPattern 一致）。
export const DEVICE_ID_RE = /^HID-[A-Z0-9]{8}$/;
