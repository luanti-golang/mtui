import { protected_fetch } from "./util.js";

export const get_settings = () => protected_fetch(`api/mapcleaner/settings`);

export const set_settings = settings => protected_fetch(`api/mapcleaner/settings`, {
    method: "POST",
    body: JSON.stringify(settings)
});

export const get_status = () => protected_fetch(`api/mapcleaner/status`);

export const start = () => protected_fetch(`api/mapcleaner/start`, { method: "POST" });

export const pause = () => protected_fetch(`api/mapcleaner/pause`, { method: "POST" });

export const reset = () => protected_fetch(`api/mapcleaner/reset`, { method: "POST" });
