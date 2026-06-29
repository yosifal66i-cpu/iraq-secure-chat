import { create } from 'zustand';
import type { Message } from './chatStore';

interface MessageState {
  messagesByChat: Record<string, Message[]>;
  pendingMessages: Record<string, Message[]>;

  addMessage: (chatId: string, message: Message) => void;
  addMessages: (chatId: string, messages: Message[]) => void;
  updateMessage: (chatId: string, messageId: string, updates: Partial<Message>) => void;
  removeMessage: (chatId: string, messageId: string) => void;
  addPendingMessage: (chatId: string, message: Message) => void;
  removePendingMessage: (chatId: string, tempId: string) => void;
  clearChatMessages: (chatId: string) => void;
}

export const useMessageStore = create<MessageState>((set) => ({
  messagesByChat: {},
  pendingMessages: {},

  addMessage: (chatId, message) =>
    set((state) => {
      const existing = state.messagesByChat[chatId] || [];
      const exists = existing.some((m) => m.message_id === message.message_id);
      if (exists) {
        return {
          messagesByChat: {
            ...state.messagesByChat,
            [chatId]: existing.map((m) =>
              m.message_id === message.message_id ? { ...m, ...message } : m,
            ),
          },
        };
      }
      return {
        messagesByChat: {
          ...state.messagesByChat,
          [chatId]: [...existing, message].sort(
            (a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime(),
          ),
        },
      };
    }),

  addMessages: (chatId, messages) =>
    set((state) => {
      const existing = state.messagesByChat[chatId] || [];
      const merged = [...existing];
      for (const msg of messages) {
        if (!merged.some((m) => m.message_id === msg.message_id)) {
          merged.push(msg);
        }
      }
      merged.sort((a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime());
      return {
        messagesByChat: { ...state.messagesByChat, [chatId]: merged },
      };
    }),

  updateMessage: (chatId, messageId, updates) =>
    set((state) => ({
      messagesByChat: {
        ...state.messagesByChat,
        [chatId]: (state.messagesByChat[chatId] || []).map((m) =>
          m.message_id === messageId ? { ...m, ...updates } : m,
        ),
      },
    })),

  removeMessage: (chatId, messageId) =>
    set((state) => ({
      messagesByChat: {
        ...state.messagesByChat,
        [chatId]: (state.messagesByChat[chatId] || []).filter(
          (m) => m.message_id !== messageId,
        ),
      },
    })),

  addPendingMessage: (chatId, message) =>
    set((state) => ({
      pendingMessages: {
        ...state.pendingMessages,
        [chatId]: [...(state.pendingMessages[chatId] || []), message],
      },
    })),

  removePendingMessage: (chatId, tempId) =>
    set((state) => ({
      pendingMessages: {
        ...state.pendingMessages,
        [chatId]: (state.pendingMessages[chatId] || []).filter(
          (m) => m.message_id !== tempId,
        ),
      },
    })),

  clearChatMessages: (chatId) =>
    set((state) => ({
      messagesByChat: { ...state.messagesByChat, [chatId]: [] },
      pendingMessages: { ...state.pendingMessages, [chatId]: [] },
    })),
}));
