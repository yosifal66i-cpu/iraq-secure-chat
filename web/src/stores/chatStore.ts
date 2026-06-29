import { create } from 'zustand';

interface ChatState {
  chats: Chat[];
  selectedChatId: string | null;
  unreadCounts: Record<string, number>;
  loading: boolean;

  setChats: (chats: Chat[]) => void;
  addChat: (chat: Chat) => void;
  updateChat: (chatId: string, updates: Partial<Chat>) => void;
  setSelectedChat: (chatId: string | null) => void;
  setUnreadCount: (chatId: string, count: number) => void;
  incrementUnread: (chatId: string) => void;
  clearUnread: (chatId: string) => void;
  setLoading: (loading: boolean) => void;
}

export interface Chat {
  id: string;
  type: 'private' | 'group' | 'supergroup' | 'channel' | 'bot';
  title?: string;
  username?: string;
  description?: string;
  avatar_url?: string;
  created_by: string;
  settings: Record<string, any>;
  created_at: string;
  last_message?: Message;
  unread_count?: number;
}

export interface Message {
  message_id: string;
  chat_id: string;
  sender_id: string;
  type: string;
  content?: string;
  reply_to?: string;
  media?: any;
  poll?: any;
  edited_at?: string;
  reactions?: Record<string, string[]>;
  entities?: any[];
  sent_at: string;
  status?: 'sending' | 'sent' | 'delivered' | 'read' | 'failed';
}

export const useChatStore = create<ChatState>((set) => ({
  chats: [],
  selectedChatId: null,
  unreadCounts: {},
  loading: false,

  setChats: (chats) => set({ chats }),

  addChat: (chat) =>
    set((state) => {
      const exists = state.chats.find((c) => c.id === chat.id);
      if (exists) {
        return {
          chats: state.chats.map((c) => (c.id === chat.id ? { ...c, ...chat } : c)),
        };
      }
      return { chats: [chat, ...state.chats] };
    }),

  updateChat: (chatId, updates) =>
    set((state) => ({
      chats: state.chats.map((c) => (c.id === chatId ? { ...c, ...updates } : c)),
    })),

  setSelectedChat: (chatId) => set({ selectedChatId: chatId }),

  setUnreadCount: (chatId, count) =>
    set((state) => ({
      unreadCounts: { ...state.unreadCounts, [chatId]: count },
    })),

  incrementUnread: (chatId) =>
    set((state) => ({
      unreadCounts: {
        ...state.unreadCounts,
        [chatId]: (state.unreadCounts[chatId] || 0) + 1,
      },
    })),

  clearUnread: (chatId) =>
    set((state) => ({
      unreadCounts: { ...state.unreadCounts, [chatId]: 0 },
    })),

  setLoading: (loading) => set({ loading }),
}));
