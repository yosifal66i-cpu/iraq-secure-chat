import axios, { AxiosError, InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '../stores/authStore';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8090';

export const apiClient = axios.create({
  baseURL: `${API_BASE_URL}/v1`,
  timeout: 30_000,
  headers: {
    'Content-Type': 'application/json',
  },
});

apiClient.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = useAuthStore.getState().accessToken;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }

  const idempotencyKey = (config as any).idempotencyKey;
  if (idempotencyKey) {
    config.headers['X-Idempotency-Key'] = idempotencyKey;
  }

  return config;
});

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean };

    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;
      try {
        const refreshToken = useAuthStore.getState().refreshToken;
        if (!refreshToken) throw new Error('No refresh token');

        const { data } = await axios.post(`${API_BASE_URL}/v1/auth/refresh`, {
          refresh_token: refreshToken,
        });

        if (data.ok) {
          useAuthStore.getState().setTokens(data.data.access_token, data.data.refresh_token);
          originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`;
          return apiClient(originalRequest);
        }
      } catch {
        useAuthStore.getState().logout();
        window.location.href = '/login';
      }
    }

    return Promise.reject(error);
  },
);

export interface ApiResponse<T = any> {
  ok: boolean;
  data?: T;
  error?: { code: number; message: string };
}

export interface PaginatedResponse<T> {
  items: T[];
  next_cursor?: string;
  has_more: boolean;
}

// Auth API
export const authApi = {
  sendOTP: (phone: string) =>
    apiClient.post<ApiResponse>('/auth/send-otp', { phone }).then(r => r.data),

  verifyOTP: (phone: string, otp: string, deviceInfo?: string) =>
    apiClient.post<ApiResponse<{
      user_id: string;
      access_token: string;
      refresh_token: string;
      expires_in: number;
      is_new_user: boolean;
      session_id: string;
    }>>('/auth/verify-otp', { phone, otp, device_info: deviceInfo }).then(r => r.data),

  refresh: (refreshToken: string) =>
    apiClient.post<ApiResponse>('/auth/refresh', { refresh_token: refreshToken }).then(r => r.data),

  logout: (sessionId?: string) =>
    apiClient.post<ApiResponse>(`/auth/logout${sessionId ? `?session_id=${sessionId}` : ''}`).then(r => r.data),

  enable2FA: (password: string, hint?: string) =>
    apiClient.post<ApiResponse>('/auth/enable-2fa', { password, hint }).then(r => r.data),

  getSessions: () =>
    apiClient.get<ApiResponse<{ sessions: any[] }>>('/auth/sessions').then(r => r.data),
};

// Users API
export const usersApi = {
  getMe: () =>
    apiClient.get<ApiResponse>('/users/me').then(r => r.data),

  updateProfile: (data: { display_name?: string; bio?: string; username?: string }) =>
    apiClient.put<ApiResponse>('/users/me', data).then(r => r.data),

  getUser: (id: string) =>
    apiClient.get<ApiResponse>(`/users/${id}`).then(r => r.data),

  searchUsers: (query: string, limit = 20) =>
    apiClient.get<ApiResponse>(`/users/search?q=${encodeURIComponent(query)}&limit=${limit}`).then(r => r.data),

  addContact: (userId: string) =>
    apiClient.post<ApiResponse>(`/users/${userId}/contact`).then(r => r.data),

  removeContact: (userId: string) =>
    apiClient.delete<ApiResponse>(`/users/${userId}/contact`).then(r => r.data),

  blockUser: (userId: string) =>
    apiClient.post<ApiResponse>(`/users/${userId}/block`).then(r => r.data),
};

// Chats API
export const chatsApi = {
  listChats: (cursor?: string, limit = 50) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    return apiClient.get<ApiResponse<PaginatedResponse<any>>>(`/chats?${params}`).then(r => r.data);
  },

  createGroup: (data: { title: string; user_ids: string[]; description?: string }) =>
    apiClient.post<ApiResponse>('/chats/create-group', data).then(r => r.data),

  createChannel: (data: { title: string; description?: string; username?: string }) =>
    apiClient.post<ApiResponse>('/chats/create-channel', data).then(r => r.data),

  getChat: (chatId: string) =>
    apiClient.get<ApiResponse>(`/chats/${chatId}`).then(r => r.data),

  updateChat: (chatId: string, data: Record<string, any>) =>
    apiClient.put<ApiResponse>(`/chats/${chatId}`, data).then(r => r.data),

  getMembers: (chatId: string, role?: string, cursor?: string, limit = 50) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (role) params.set('role', role);
    if (cursor) params.set('cursor', cursor);
    return apiClient.get<ApiResponse<PaginatedResponse<any>>>(`/chats/${chatId}/members?${params}`).then(r => r.data);
  },

  addMember: (chatId: string, userId: string, role?: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/members`, { user_id: userId, role }).then(r => r.data),

  removeMember: (chatId: string, userId: string) =>
    apiClient.delete<ApiResponse>(`/chats/${chatId}/members/${userId}`).then(r => r.data),

  createInviteLink: (chatId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/invite-link`).then(r => r.data),

  joinChat: (chatId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/join`).then(r => r.data),

  leaveChat: (chatId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/leave`).then(r => r.data),
};

// Messages API
export const messagesApi = {
  getMessages: (chatId: string, cursor?: string, direction = 'before', limit = 50) => {
    const params = new URLSearchParams({ direction, limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    return apiClient.get<ApiResponse<PaginatedResponse<any>>>(`/chats/${chatId}/messages?${params}`).then(r => r.data);
  },

  sendMessage: (chatId: string, data: {
    type: string;
    text?: string;
    media_id?: string;
    reply_to?: string;
    idempotency_key?: string;
  }) => {
    const idempotencyKey = data.idempotency_key || crypto.randomUUID();
    return apiClient.post<ApiResponse<{ message_id: string; sent_at: string }>>(
      `/chats/${chatId}/messages`,
      { ...data, idempotency_key: idempotencyKey },
      { headers: { 'X-Idempotency-Key': idempotencyKey } } as any,
    ).then(r => r.data);
  },

  editMessage: (chatId: string, msgId: string, text: string) =>
    apiClient.put<ApiResponse>(`/chats/${chatId}/messages/${msgId}`, { text }).then(r => r.data),

  deleteMessage: (chatId: string, msgId: string, forEveryone = false) =>
    apiClient.delete<ApiResponse>(`/chats/${chatId}/messages/${msgId}?for_everyone=${forEveryone}`).then(r => r.data),

  reactToMessage: (chatId: string, msgId: string, emoji: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/messages/${msgId}/react`, { emoji }).then(r => r.data),

  pinMessage: (chatId: string, msgId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/messages/${msgId}/pin`).then(r => r.data),

  markRead: (chatId: string, maxMessageId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/messages/read`, { max_message_id: maxMessageId }).then(r => r.data),

  searchMessages: (chatId: string, query: string, cursor?: string, limit = 50) => {
    const params = new URLSearchParams({ q: query, limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    return apiClient.get<ApiResponse<PaginatedResponse<any>>>(`/chats/${chatId}/messages/search?${params}`).then(r => r.data);
  },

  forwardMessage: (chatId: string, msgId: string, toChatId: string) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/messages/${msgId}/forward`, { to_chat_id: toChatId }).then(r => r.data),

  setAutoDelete: (chatId: string, msgId: string, ttl: number) =>
    apiClient.post<ApiResponse>(`/chats/${chatId}/messages/${msgId}/auto-delete`, { ttl }).then(r => r.data),
};

// Media API
export const mediaApi = {
  upload: (file: File, onProgress?: (pct: number) => void) => {
    const formData = new FormData();
    formData.append('file', file);
    return apiClient.post<ApiResponse>('/media/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (onProgress && e.total) onProgress(Math.round((e.loaded * 100) / e.total));
      },
      timeout: 5 * 60 * 1000,
    }).then(r => r.data);
  },

  getMedia: (mediaId: string) =>
    apiClient.get<ApiResponse>(`/media/${mediaId}`).then(r => r.data),

  deleteMedia: (mediaId: string) =>
    apiClient.delete<ApiResponse>(`/media/${mediaId}`).then(r => r.data),
};

// AI API
export const aiApi = {
  translate: (text: string, targetLang: string, sourceLang?: string) =>
    apiClient.post<ApiResponse<{ translated_text: string; source_lang: string; target_lang: string }>>('/ai/translate', {
      text, target_lang: targetLang, source_lang: sourceLang,
    }).then(r => r.data),

  moderate: (text: string) =>
    apiClient.post<ApiResponse>('/ai/moderate', { text }).then(r => r.data),

  suggestReply: (messages: string[], count = 3) =>
    apiClient.post<ApiResponse<{ replies: string[] }>>('/ai/suggest-reply', { messages, count }).then(r => r.data),

  summarize: (text: string, maxWords = 100) =>
    apiClient.post<ApiResponse<{ summary: string }>>('/ai/summarize', { text, max_words: maxWords }).then(r => r.data),

  complete: (prompt: string, maxTokens = 500, temperature = 0.7) =>
    apiClient.post<ApiResponse<{ text: string }>>('/ai/complete', {
      prompt, max_tokens: maxTokens, temperature,
    }).then(r => r.data),
};
