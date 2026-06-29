import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { chatsApi } from '../api/client';
import { useChatStore } from '../stores/chatStore';
import { useAuthStore } from '../stores/authStore';
import type { Chat } from '../stores/chatStore';

export default function ChatListPage() {
  const navigate = useNavigate();
  const { chats, setChats, selectedChatId, setSelectedChat, loading, setLoading } = useChatStore();
  const user = useAuthStore((state) => state.user);
  const [search, setSearch] = useState('');

  useEffect(() => {
    loadChats();
  }, []);

  const loadChats = async () => {
    setLoading(true);
    try {
      const res = await chatsApi.listChats();
      if (res.ok && res.data) {
        setChats(res.data.items || []);
      }
    } catch (err) {
      console.error('Failed to load chats:', err);
    } finally {
      setLoading(false);
    }
  };

  const filteredChats = search
    ? chats.filter((c) =>
        c.title?.toLowerCase().includes(search.toLowerCase()),
      )
    : chats;

  const handleChatClick = (chatId: string) => {
    setSelectedChat(chatId);
    navigate(`/chat/${chatId}`);
  };

  const getChatTitle = (chat: Chat) => {
    if (chat.title) return chat.title;
    if (chat.type === 'private') return 'User';
    return 'Unknown';
  };

  const getChatSubtitle = (chat: Chat) => {
    if (chat.type === 'private') return chat.description || 'Private chat';
    if (chat.type === 'channel') return chat.description || 'Channel';
    if (chat.type === 'group' || chat.type === 'supergroup') return chat.description || 'Group';
    return '';
  };

  const getAvatarLetters = (name: string) => {
    return name.charAt(0).toUpperCase();
  };

  return (
    <div className="flex-1 flex flex-col max-w-md border-l border-gray-200 dark:border-gray-700">
      {/* Header */}
      <div className="p-4 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white">IraqSecureChat</h1>
      </div>

      {/* Search */}
      <div className="p-3 bg-white dark:bg-gray-800">
        <div className="relative">
          <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search chats..."
            className="w-full pl-10 pr-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg
              text-sm text-gray-900 dark:text-white placeholder-gray-400
              focus:ring-2 focus:ring-primary focus:bg-white dark:focus:bg-gray-600 outline-none transition-all"
          />
        </div>
      </div>

      {/* Chat list */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center h-32">
            <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          </div>
        ) : filteredChats.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-gray-400">
            <svg className="w-12 h-12 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
            </svg>
            <p className="text-sm">No chats yet</p>
          </div>
        ) : (
          filteredChats.map((chat) => (
            <button
              key={chat.id}
              onClick={() => handleChatClick(chat.id)}
              className={`w-full flex items-center gap-3 px-4 py-3 transition-colors
                hover:bg-gray-100 dark:hover:bg-gray-700/50 border-b border-gray-100 dark:border-gray-700/50
                ${selectedChatId === chat.id ? 'bg-primary/5' : ''}`}
            >
              {/* Avatar */}
              <div className={`w-12 h-12 rounded-full flex items-center justify-center text-white font-bold shrink-0
                ${chat.type === 'channel' ? 'bg-green-500' : chat.type === 'group' || chat.type === 'supergroup' ? 'bg-orange-500' : 'bg-primary'}`}>
                {getAvatarLetters(getChatTitle(chat))}
              </div>

              {/* Info */}
              <div className="flex-1 min-w-0 text-right">
                <div className="flex items-center justify-between">
                  <h3 className="font-semibold text-sm text-gray-900 dark:text-white truncate">
                    {getChatTitle(chat)}
                  </h3>
                  <span className="text-xs text-gray-400 shrink-0 mr-2">now</span>
                </div>
                <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">
                  {getChatSubtitle(chat)}
                </p>
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}
