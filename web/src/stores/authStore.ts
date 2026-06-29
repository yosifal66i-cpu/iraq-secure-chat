import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  userId: string | null;
  accessToken: string | null;
  refreshToken: string | null;
  sessionId: string | null;
  isAuthenticated: boolean;
  user: UserProfile | null;

  setAuth: (data: {
    user_id: string;
    access_token: string;
    refresh_token: string;
    session_id: string;
    user?: UserProfile;
  }) => void;
  setTokens: (accessToken: string, refreshToken: string) => void;
  setUser: (user: UserProfile) => void;
  logout: () => void;
}

export interface UserProfile {
  id: string;
  phone?: string;
  email?: string;
  username?: string;
  display_name: string;
  bio?: string;
  avatar_url?: string;
  premium: boolean;
  last_seen: string;
  created_at: string;
  settings?: Record<string, any>;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      userId: null,
      accessToken: null,
      refreshToken: null,
      sessionId: null,
      isAuthenticated: false,
      user: null,

      setAuth: (data) =>
        set({
          userId: data.user_id,
          accessToken: data.access_token,
          refreshToken: data.refresh_token,
          sessionId: data.session_id,
          isAuthenticated: true,
          user: data.user || null,
        }),

      setTokens: (accessToken, refreshToken) =>
        set({ accessToken, refreshToken }),

      setUser: (user) =>
        set({ user }),

      logout: () =>
        set({
          userId: null,
          accessToken: null,
          refreshToken: null,
          sessionId: null,
          isAuthenticated: false,
          user: null,
        }),
    }),
    {
      name: 'iraqchat-auth',
      partialize: (state) => ({
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
        userId: state.userId,
        sessionId: state.sessionId,
        isAuthenticated: state.isAuthenticated,
        user: state.user,
      }),
    },
  ),
);
