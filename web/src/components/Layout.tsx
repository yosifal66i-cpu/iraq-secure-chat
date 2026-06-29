import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { useAuthStore } from '../stores/authStore';
import { usersApi } from '../api/client';
import { useEffect, useState } from 'react';

export default function Layout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, setUser, logout } = useAuthStore();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  useEffect(() => {
    if (!user) {
      usersApi.getMe().then((res) => {
        if (res.ok && res.data) setUser(res.data);
      }).catch(() => logout());
    }
  }, []);

  const isChatPage = location.pathname.startsWith('/chat/');

  return (
    <div className="flex h-screen overflow-hidden bg-gray-50 dark:bg-gray-900">
      {/* Sidebar */}
      <aside className="w-20 lg:w-64 bg-white dark:bg-gray-800 border-l border-gray-200 dark:border-gray-700 flex flex-col shrink-0">
        <div className="p-4 border-b border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-primary flex items-center justify-center text-white font-bold text-lg">
              {user?.display_name?.charAt(0) || '?'}
            </div>
            <div className="hidden lg:block">
              <p className="font-semibold text-sm truncate">{user?.display_name || 'User'}</p>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {user?.username ? `@${user.username}` : 'Online'}
              </p>
            </div>
          </div>
        </div>

        <nav className="flex-1 p-2 space-y-1">
          <button
            onClick={() => { navigate('/'); setMobileNavOpen(false); }}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors
              ${!isChatPage ? 'bg-primary/10 text-primary' : 'hover:bg-gray-100 dark:hover:bg-gray-700'}`}
          >
            <svg className="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
            </svg>
            <span className="hidden lg:inline">Chats</span>
          </button>

          <button
            onClick={() => navigate('/settings')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors
              ${location.pathname === '/settings' ? 'bg-primary/10 text-primary' : 'hover:bg-gray-100 dark:hover:bg-gray-700'}`}
          >
            <svg className="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            <span className="hidden lg:inline">Settings</span>
          </button>
        </nav>

        <div className="p-4 border-t border-gray-200 dark:border-gray-700">
          <button
            onClick={() => {
              logout();
              navigate('/login');
            }}
            className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
          >
            <svg className="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
            </svg>
            <span className="hidden lg:inline">Logout</span>
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 flex overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
