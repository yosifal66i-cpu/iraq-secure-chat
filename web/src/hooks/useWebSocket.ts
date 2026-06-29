import { useEffect, useRef, useCallback } from 'react';
import { useAuthStore } from '../stores/authStore';

type WSEventHandler = (data: any) => void;

const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8090/v1/ws';

export function useWebSocket(handlers: Record<string, WSEventHandler>) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number>();
  const reconnectAttempts = useRef(0);
  const accessToken = useAuthStore((state) => state.accessToken);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const connect = useCallback(() => {
    if (!accessToken || !isAuthenticated) return;

    const url = `${WS_URL}?token=${accessToken}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttempts.current = 0;
    };

    ws.onmessage = (event) => {
      try {
        const frame = JSON.parse(event.data);
        const handler = handlers[frame.type];
        if (handler) {
          handler(frame.data);
        }
      } catch (err) {
        console.error('WebSocket message parse error:', err);
      }
    };

    ws.onclose = () => {
      wsRef.current = null;
      // Exponential backoff reconnection
      const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000);
      reconnectAttempts.current++;
      reconnectTimeoutRef.current = window.setTimeout(connect, delay);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [accessToken, isAuthenticated, handlers]);

  const send = useCallback((frame: { type: string; [key: string]: any }) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(frame));
    }
  }, []);

  const subscribe = useCallback((chatIds: string[]) => {
    send({ type: 'subscribe', chat_ids: chatIds });
  }, [send]);

  const sendTyping = useCallback((chatId: string, action: 'start' | 'stop') => {
    send({ type: 'typing', chat_id: chatId, action });
  }, [send]);

  const sendRead = useCallback((chatId: string, messageId: string) => {
    send({ type: 'read', chat_id: chatId, message_id: messageId });
  }, [send]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      wsRef.current?.close();
    };
  }, [connect]);

  return { send, subscribe, sendTyping, sendRead, isConnected: !!wsRef.current };
}
