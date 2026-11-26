-- Back to strict containment.
CREATE OR REPLACE FUNCTION check_ip_in_segment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM segments WHERE id = NEW.segment_id AND NEW.ip << network) THEN
    RAISE EXCEPTION 'IP % is outside its segment', NEW.ip USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION check_range_in_segment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM segments WHERE id = NEW.segment_id AND NEW.start_ip << network AND NEW.end_ip << network) THEN
    RAISE EXCEPTION 'range %-% is outside its segment', NEW.start_ip, NEW.end_ip USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION check_segment_contents() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM ip_addresses WHERE segment_id = NEW.id AND NOT (ip << NEW.network))
     OR EXISTS (SELECT 1 FROM reservations WHERE segment_id = NEW.id AND status = 'active'
                AND NOT (start_ip << NEW.network AND end_ip << NEW.network)) THEN
    RAISE EXCEPTION 'segment % no longer holds its addresses', NEW.network USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
