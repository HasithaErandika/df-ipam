-- Removes the policies only. Row level security stays on deliberately: rolling
-- back must never reopen these tables to the public Data API.
DO $$
DECLARE t text;
BEGIN
  FOR t IN SELECT tablename FROM pg_policies WHERE schemaname = 'public' AND policyname = 'dfipam_app_all' LOOP
    EXECUTE format('DROP POLICY dfipam_app_all ON public.%I', t);
  END LOOP;
END $$;
