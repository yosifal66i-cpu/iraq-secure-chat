import { useEffect, useState, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Virtuoso, VirtuosoHandle } from 'react-virtuoso';
import toast from 'react-hot-toast';
import { chatsApi, messagesApi } from '../api/client';
import { useAuthStore } from '../stores/authStore';
import { useChatStore, type Message } from '../stores/chatStore';
import { useMessageStore } from '../stores/messageStore';
import { useWebSocket } from '../hooks/useWebSocket';

export default function ChatPage() {
  const { chatId } = useParams<{ chatId: string }>();
  const navigate = useNavigate();
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const [inputText, setInputText] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [cursor, setCursor] = useState<string | undefined>();
  const [hasMore, setHasMore] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [chat, setChat] = useState<any>(null);
  const [replyTo, setReplyTo] = useState<string | undefined>();

  const user = useAuthStore((state) => state.user);
  const { messagesByChat, addMessage, addMessages, updateMessage, pendingMessages } = useMessageStore();
  const { addChat, unreadCounts, clearUnread } = useChatStore();

  const messages = messagesByChat[chatId || ''] || [];
  const pending = pendingMessages[chatId || ''] || [];

  const handleWSMessage = useCallback((type: string) => (data: any) => {
    if (!chatId) return;
    switch (type) {
      case 'message.new':
        addMessage(chatId, data);
        break;
      case 'message.edited':
        updateMessage(chatId, data.message_id, { content: data.new_text, edited_at: data.edited_at });
        break;
      case 'message.deleted':
        if (data.for_everyone) {
          updateMessage(chatId, data.message_id, { content: '[deleted]' });
        }
        break;
      case 'reaction.add':
        updateMessage(chatId, data.message_id, {
          reactions: { ...(messagesByChat[chatId]?.find(m => m.message_id === data.message_id)?.reactions || {}), [data.emoji]: [data.user_id] }
        });
        break;
    }
  }, [chatId, addMessage, updateMessage]);

  const ws = useWebSocket({
    'message.new': handleWSMessage('message.new'),
    'message.edited': handleWSMessage('message.edited'),
    'message.deleted': handleWSMessage('message.deleted'),
    'reaction.add': handleWSMessage('reaction.add'),
  });

  useEffect(() => {
    if (chatId) {
      loadChat();
      loadMessages();
      ws.subscribe([chatId]);
      clearUnread(chatId);
    }
  }, [chatId]);

  const loadChat = async () => {
    if (!chatId) return;
    try {
      const res = await chatsApi.getChat(chatId);
      if (res.ok && res.data) {
        setChat(res.data);
        addChat(res.data);
      }
    } catch (err) {
      toast.error('Failed to load chat');
      navigate('/');
    }
  };

  const loadMessages = async () => {
    if (!chatId) return;
    setIsLoading(true);
    try {
      const res = await messagesApi.getMessages(chatId);
      if (res.ok && res.data) {
        addMessages(chatId, res.data.items || []);
        setCursor(res.data.next_cursor);
        setHasMore(res.data.has_more);
      }
    } catch (err) {
      console.error('Failed to load messages');
    } finally {
      setIsLoading(false);
    }
  };

  const loadMoreMessages = async () => {
    if (!chatId || !cursor || !hasMore || loadingMore) return;
    setLoadingMore(true);
    try {
      const res = await messagesApi.getMessages(chatId, cursor);
      if (res.ok && res.data) {
        addMessages(chatId, res.data.items || []);
        setCursor(res.data.next_cursor);
        setHasMore(res.data.has_more);
      }
    } catch (err) {
      console.error('Failed to load more');
    } finally {
      setLoadingMore(false);
    }
  };

  const handleSend = async () => {
    if (!chatId || !inputText.trim()) return;

    const text = inputText.trim();
    setInputText('');

    const tempId = `temp-${Date.now()}`;
    const pendingMsg: Message = {
      message_id: tempId,
      chat_id: chatId,
      sender_id: user?.id || '',
      type: 'text',
      content: text,
      sent_at: new Date().toISOString(),
      status: 'sending',
    };

    addMessage(chatId, pendingMsg);

    try {
      const res = await messagesApi.sendMessage(chatId, {
        type: 'text',
        text,
        reply_to: replyTo,
      });

      if (res.ok && res.data) {
        updateMessage(chatId, tempId, {
          message_id: res.data.message_id,
          status: 'sent',
          sent_at: res.data.sent_at,
        });
        setReplyTo(undefined);
      }
    } catch (err) {
      updateMessage(chatId, tempId, { status: 'failed' });
      toast.error('Failed to send message');
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleReact = async (msgId: string, emoji: string) => {
    if (!chatId) return;
    try {
      await messagesApi.reactToMessage(chatId, msgId, emoji);
    } catch (err) {
      toast.error('Failed to react');
    }
  };

  const handleDelete = async (msgId: string, forEveryone = false) => {
    if (!chatId) return;
    try {
      await messagesApi.deleteMessage(chatId, msgId, forEveryone);
      if (forEveryone) {
        updateMessage(chatId, msgId, { content: '[deleted]' });
      } else {
        useMessageStore.getState().removeMessage(chatId, msgId);
      }
    } catch (err) {
      toast.error('Failed to delete');
    }
  };

  const formatTime = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toLocaleTimeString('ar-IQ', { hour: '2-digit', minute: '2-digit' });
  };

  const isOwnMessage = (senderId: string) => senderId === user?.id;

  if (!chat) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col bg-white dark:bg-gray-800">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex items-center gap-3 bg-white dark:bg-gray-800">
        <button onClick={() => navigate('/')} className="lg:hidden p-1 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <div className="w-10 h-10 rounded-full bg-primary flex items-center justify-center text-white font-bold">
          {chat.title?.charAt(0) || '?'}
        </div>
        <div>
          <h2 className="font-semibold text-sm">{chat.title || 'Chat'}</h2>
          <p className="text-xs text-gray-400">{chat.type === 'channel' ? 'Channel' : chat.type === 'private' ? 'Private' : 'Group'}</p>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-hidden bg-gray-50 dark:bg-gray-900/50">
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <Virtuoso
            ref={virtuosoRef}
            className="h-full"
            data={messages}
            followOutput="smooth"
            startReached={loadMoreMessages}
            itemContent={(index, message) => {
              const own = isOwnMessage(message.sender_id);
              const failed = message.status === 'failed';
              const sending = message.status === 'sending';

              return (
                <div className={`px-4 py-1 flex ${own ? 'justify-end' : 'justify-start'}`}>
                  <div className={`max-w-[75%] group ${own ? 'order-1' : 'order-0'}`}>
                    <div className={`px-3 py-2 rounded-2xl text-sm ${
                      own
                        ? 'bg-primary text-white rounded-br-sm'
                        : 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white rounded-bl-sm shadow-sm'
                    } ${failed ? 'opacity-70' : ''}`}>
                      {message.content && <p className="whitespace-pre-wrap break-words">{message.content}</p>}
                      {message.media && <p className="text-xs opacity-75 mt-1">[Media]</p>}
                      <div className={`flex items-center gap-1 mt-1 ${own ? 'justify-end' : 'justify-start'}`}>
                        <span className="text-[10px] opacity-60">{formatTime(message.sent_at)}</span>
                        {own && (
                          <span className="text-[10px]">
                            {sending ? '⏳' : failed ? '⚠️' : '✓'}
                          </span>
                        )}
                      </div>
                    </div>

                    {/* Reactions */}
                    {message.reactions && Object.keys(message.reactions).length > 0 && (
                      <div className={`flex gap-1 mt-1 ${own ? 'justify-end' : 'justify-start'}`}>
                        {Object.entries(message.reactions).map(([emoji, users]) => (
                          <button
                            key={emoji}
                            onClick={() => handleReact(message.message_id, emoji)}
                            className={`text-xs px-2 py-0.5 rounded-full border ${
                              users.includes(user?.id || '')
                                ? 'bg-primary/10 border-primary/30'
                                : 'bg-gray-100 dark:bg-gray-700 border-gray-200 dark:border-gray-600'
                            }`}
                          >
                            {emoji} {users.length}
                          </button>
                        ))}
                      </div>
                    )}

                    {/* Actions */}
                    {own && !sending && (
                      <div className={`hidden group-hover:flex gap-1 mt-1 ${own ? 'justify-end' : 'justify-start'}`}>
                        <button onClick={() => handleReact(message.message_id, '👍')} className="text-xs p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded">
                          👍
                        </button>
                        <button onClick={() => handleDelete(message.message_id)} className="text-xs p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded text-red-500">
                          🗑️
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              );
            }}
          />
        )}
      </div>

      {/* Reply indicator */}
      {replyTo && (
        <div className="px-4 py-2 bg-gray-100 dark:bg-gray-700 flex items-center gap-2 text-sm">
          <span className="text-primary">Replying...</span>
          <button onClick={() => setReplyTo(undefined)} className="text-gray-400 hover:text-gray-600">
            ✕
          </button>
        </div>
      )}

      {/* Input */}
      <div className="px-4 py-3 border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="flex items-end gap-2">
          <textarea
            ref={inputRef}
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            rows={1}
            className="flex-1 px-4 py-2.5 bg-gray-100 dark:bg-gray-700 rounded-xl resize-none
              text-sm text-gray-900 dark:text-white placeholder-gray-400
              focus:ring-2 focus:ring-primary focus:bg-white dark:focus:bg-gray-600 outline-none transition-all
              max-h-32"
          />
          <button
            onClick={handleSend}
            disabled={!inputText.trim()}
            className="p-2.5 bg-primary text-white rounded-xl hover:bg-primary-dark transition-colors
              disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M12 19V5m0 0l-7 7m7-7l7 7" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}
