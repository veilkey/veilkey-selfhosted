CREATE INDEX IF NOT EXISTS idx_functions_scope ON functions(scope);
CREATE INDEX IF NOT EXISTS idx_functions_created_at ON functions(created_at);
CREATE INDEX IF NOT EXISTS idx_function_logs_function_hash ON function_logs(function_hash);
