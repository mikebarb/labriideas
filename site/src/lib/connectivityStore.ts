import { writable, type Readable } from 'svelte/store';

const isOnlineWritable = writable<boolean>(typeof navigator === 'undefined' ? true : navigator.onLine);
let listenersAttached = false;

function attachListeners() {
  if (listenersAttached || typeof window === 'undefined') return;
  listenersAttached = true;

  isOnlineWritable.set(navigator.onLine);

  window.addEventListener('online', () => {
    isOnlineWritable.set(true);
  });

  window.addEventListener('offline', () => {
    isOnlineWritable.set(false);
  });
}

if (typeof window !== 'undefined') {
  attachListeners();
}

export const isOnline: Readable<boolean> = {
  subscribe: (run) => {
    return isOnlineWritable.subscribe(run);
  }
};

export async function verifyConnectivity(url = '/favicon.ico'): Promise<boolean> {
  if (typeof window === 'undefined') return false;
  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 5000);

    const res = await fetch(url, {
      method: 'HEAD',
      cache: 'no-store',
      signal: controller.signal
    });

    clearTimeout(timeoutId);
    const online = res.ok;
    isOnlineWritable.set(online);
    return online;
  } catch {
    isOnlineWritable.set(false);
    return false;
  }
}
