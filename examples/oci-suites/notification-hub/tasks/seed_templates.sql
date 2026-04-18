INSERT INTO notification_templates (name, channel, subject, body) VALUES
  ('welcome_email',        'email', 'Welcome to the platform', 'Hi {{name}}, your account is ready.'),
  ('password_reset',       'email', 'Reset your password',    'Use this link to reset: {{reset_url}}'),
  ('order_shipped',        'email', 'Your order is on its way', 'Order {{order_id}} has shipped.'),
  ('sms_otp',              'sms',   NULL,                     'Your one-time code is {{otp}}. Expires in 10 minutes.'),
  ('push_new_message',     'push',  'New message',            '{{sender}} sent you a message.')
ON CONFLICT (name) DO NOTHING;
