// Admin Portal config（CL-5c）。独立于用户门户（portal/），用独立 token key。
// API_BASE 与 portal 一致（同源 /api/v1）；仅 token 隔离。

const override = localStorage.getItem('smarthid_cloud_base');
export const API_BASE = override || '/api/v1';
export const TOKEN_KEY = 'smarthid_admin_token';
export const USER_KEY = 'smarthid_admin_user';
