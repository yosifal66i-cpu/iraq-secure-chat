import { useState } from 'react';
import toast from 'react-hot-toast';
import { usersApi, authApi } from '../api/client';
import { useAuthStore } from '../stores/authStore';

export default function SettingsPage() {
  const { user, setUser } = useAuthStore();
  const [displayName, setDisplayName] = useState(user?.display_name || '');
  const [bio, setBio] = useState(user?.bio || '');
  const [username, setUsername] = useState(user?.username || '');
  const [saving, setSaving] = useState(false);
  const [show2FA, setShow2FA] = useState(false);
  const [password, setPassword] = useState('');
  const [hint, setHint] = useState('');

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      const res = await usersApi.updateProfile({
        display_name: displayName,
        bio: bio,
        username: username,
      });
      if (res.ok && res.data) {
        setUser(res.data);
        toast.success('Profile updated');
      }
    } catch (err: any) {
      toast.error(err?.response?.data?.error?.message || 'Failed to update');
    } finally {
      setSaving(false);
    }
  };

  const handleEnable2FA = async () => {
    if (password.length < 8) {
      toast.error('Password must be at least 8 characters');
      return;
    }
    try {
      const res = await authApi.enable2FA(password, hint);
      if (res.ok) {
        toast.success('2FA enabled');
        setShow2FA(false);
        setPassword('');
        setHint('');
      }
    } catch (err: any) {
      toast.error(err?.response?.data?.error?.message || 'Failed to enable 2FA');
    }
  };

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-2xl mx-auto p-6 space-y-6">
        <h1 className="text-2xl font-bold">Settings / الإعدادات</h1>

        {/* Profile */}
        <section className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">Profile / الملف الشخصي</h2>
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Display Name / الاسم
              </label>
              <input
                type="text"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg
                  bg-white dark:bg-gray-700 text-gray-900 dark:text-white
                  focus:ring-2 focus:ring-primary outline-none"
                maxLength={64}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Username / اسم المستخدم
              </label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg
                  bg-white dark:bg-gray-700 text-gray-900 dark:text-white
                  focus:ring-2 focus:ring-primary outline-none"
                maxLength={32}
                placeholder="@username"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Bio / السيرة
              </label>
              <textarea
                value={bio}
                onChange={(e) => setBio(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg
                  bg-white dark:bg-gray-700 text-gray-900 dark:text-white
                  focus:ring-2 focus:ring-primary outline-none resize-none"
                maxLength={512}
                rows={3}
              />
            </div>
            <button
              onClick={handleSaveProfile}
              disabled={saving}
              className="px-6 py-2 bg-primary text-white rounded-lg font-medium
                hover:bg-primary-dark transition-colors disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save / حفظ'}
            </button>
          </div>
        </section>

        {/* Security */}
        <section className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">Security / الأمان</h2>
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">Two-Factor Authentication</p>
                <p className="text-sm text-gray-500">Add extra security to your account</p>
              </div>
              <button
                onClick={() => setShow2FA(!show2FA)}
                className="px-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200 dark:hover:bg-gray-600"
              >
                {show2FA ? 'Cancel' : 'Enable'}
              </button>
            </div>
            {show2FA && (
              <div className="space-y-3 p-4 bg-gray-50 dark:bg-gray-900 rounded-lg">
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Cloud password (min 8 chars)"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary outline-none"
                />
                <input
                  type="text"
                  value={hint}
                  onChange={(e) => setHint(e.target.value)}
                  placeholder="Password hint (optional)"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary outline-none"
                />
                <button
                  onClick={handleEnable2FA}
                  className="px-4 py-2 bg-primary text-white rounded-lg text-sm"
                >
                  Enable 2FA
                </button>
              </div>
            )}
          </div>
        </section>

        {/* Privacy */}
        <section className="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">Privacy / الخصوصية</h2>
          <div className="space-y-3 text-sm">
            <div className="flex items-center justify-between">
              <span>Last Seen</span>
              <span className="text-gray-500">Everyone</span>
            </div>
            <div className="flex items-center justify-between">
              <span>Profile Photo</span>
              <span className="text-gray-500">Everyone</span>
            </div>
            <div className="flex items-center justify-between">
              <span>Phone Number</span>
              <span className="text-gray-500">Contacts</span>
            </div>
          </div>
        </section>

        {/* About */}
        <section className="text-center text-sm text-gray-400 py-4">
          <p>IraqSecureChat v1.0.0</p>
          <p className="mt-1">تواصل آمن للجهات الحكومية العراقية</p>
          <p className="mt-1">© 2025 IraqSecureChat. All rights reserved.</p>
        </section>
      </div>
    </div>
  );
}
