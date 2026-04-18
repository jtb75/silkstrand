package silkstrand.mssql.max_error_log_files

import rego.v1

default result := {
    "control_id": "mssql-max-error-log-files",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Maximum number of error log files is set to 12 or greater",
    "remediation": "EXEC master.sys.xp_instance_regwrite N'HKEY_LOCAL_MACHINE', N'Software\\Microsoft\\MSSQLServer\\MSSQLServer', N'NumErrorLogs', REG_DWORD, 12;"
}

result := r if {
    input.facts.max_error_log_files >= 12
    r := {
        "control_id": "mssql-max-error-log-files",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Maximum number of error log files is set to 12 or greater",
        "evidence": {"max_error_log_files": input.facts.max_error_log_files}
    }
}
