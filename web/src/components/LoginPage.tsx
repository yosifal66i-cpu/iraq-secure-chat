import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { authApi } from '../api/client';
import { useAuthStore } from '../stores/authStore';

export default function LoginPage() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.setAuth);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const [phone, setPhone] = useState('');
  const [otp, setOtp] = useState('');
  const [step, setStep] = useState<'phone' | 'otp'>('phone');
  const [loading, setLoading] = useState(false);
  const [otpExpiry, setOtpExpiry] = useState(0);

  if (isAuthenticated) {
    navigate('/', { replace: true });
  }

  const handleSendOTP = async () => {
    if (!phone || phone.length < 10) {
      toast.error('Please enter a valid phone number');
      return;
    }

    setLoading(true);
    try {
      const res = await authApi.sendOTP(phone);
      if (res.ok) {
        setStep('otp');
        setOtpExpiry(res.data.expires_in);
        toast.success('OTP sent successfully');
      }
    } catch (err: any) {
      toast.error(err?.response?.data?.error?.message || 'Failed to send OTP');
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyOTP = async () => {
    if (!otp || otp.length < 4) {
      toast.error('Please enter the verification code');
      return;
    }

    setLoading(true);
    try {
      const res = await authApi.verifyOTP(phone, otp, navigator.userAgent);
      if (res.ok && res.data) {
        setAuth(res.data);
        toast.success('Welcome to IraqSecureChat');
        navigate('/', { replace: true });
      }
    } catch (err: any) {
      toast.error(err?.response?.data?.error?.message || 'Invalid code');
    } finally {
      setLoading(false);
    }
  };

  const handlePhoneChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value.replace(/[^0-9+]/g, '');
    setPhone(val);
  };

  const handleOtpChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value.replace(/[^0-9]/g, '');
    if (val.length <= 6) setOtp(val);
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary to-primary-dark flex items-center justify-center p-4">
      <div className="w-full max-w-md bg-white dark:bg-gray-800 rounded-2xl shadow-2xl p-8">
        {/* Logo */}
        <div className="text-center mb-8">
          <div className="w-16 h-16 bg-primary rounded-2xl flex items-center justify-center mx-auto mb-4">
            <svg className="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">IraqSecureChat</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">المراسلة الآمنة للجهات الحكومية</p>
        </div>

        {step === 'phone' ? (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Phone Number / رقم الهاتف
              </label>
              <input
                type="tel"
                value={phone}
                onChange={handlePhoneChange}
                placeholder="+9647XXXXXXXX"
                className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg
                  bg-white dark:bg-gray-700 text-gray-900 dark:text-white
                  focus:ring-2 focus:ring-primary focus:border-transparent outline-none
                  text-lg dir-ltr text-left"
                dir="ltr"
                inputMode="tel"
                autoFocus
              />
              <p className="text-xs text-gray-400 mt-1">Iraqi phone number (+964)</p>
            </div>
            <button
              onClick={handleSendOTP}
              disabled={loading || phone.length < 10}
              className="w-full py-3 bg-primary text-white rounded-lg font-semibold
                hover:bg-primary-dark transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Sending...' : 'Send Code / إرسال الرمز'}
            </button>
          </div>
        ) : (
          <div className="space-y-4 animate-fade-in">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Verification Code / رمز التحقق
              </label>
              <input
                type="text"
                value={otp}
                onChange={handleOtpChange}
                placeholder="000000"
                className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg
                  bg-white dark:bg-gray-700 text-gray-900 dark:text-white
                  focus:ring-2 focus:ring-primary focus:border-transparent outline-none
                  text-2xl text-center tracking-[0.5em]"
                inputMode="numeric"
                autoFocus
                maxLength={6}
              />
              <p className="text-xs text-gray-400 mt-1 text-center">
                Code expires in {otpExpiry} seconds
              </p>
            </div>
            <button
              onClick={handleVerifyOTP}
              disabled={loading || otp.length < 4}
              className="w-full py-3 bg-primary text-white rounded-lg font-semibold
                hover:bg-primary-dark transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Verifying...' : 'Verify / تحقق'}
            </button>
            <button
              onClick={() => setStep('phone')}
              className="w-full py-2 text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 transition-colors"
            >
              Change phone number
            </button>
          </div>
        )}

        <div className="mt-6 text-center">
          <p className="text-xs text-gray-400">
            Secure communication for Iraqi government entities
          </p>
          <p className="text-xs text-gray-400 mt-1">
            تواصل آمن للجهات الحكومية العراقية
          </p>
        </div>
      </div>
    </div>
  );
}
