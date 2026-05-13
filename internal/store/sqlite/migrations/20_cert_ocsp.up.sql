ALTER TABLE cert_check_results ADD COLUMN ocsp_stapled INTEGER;
ALTER TABLE cert_check_results ADD COLUMN ocsp_status TEXT;
ALTER TABLE cert_check_results ADD COLUMN ocsp_produced_at INTEGER;
ALTER TABLE cert_check_results ADD COLUMN ocsp_next_update INTEGER;
ALTER TABLE cert_check_results ADD COLUMN ocsp_error TEXT;
