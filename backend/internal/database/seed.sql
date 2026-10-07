INSERT INTO admins (username, password_hash, display_name, role)
VALUES ('admin', '$2a$10$KNFOxy/nnJyeCFCDx6keI.ocYSbPiNTtG583zEYpPYwwxzhxAt8HK', '系统管理员', 'super_admin')
ON DUPLICATE KEY UPDATE
    password_hash = '$2a$10$KNFOxy/nnJyeCFCDx6keI.ocYSbPiNTtG583zEYpPYwwxzhxAt8HK',
    display_name = '系统管理员',
    role = 'super_admin',
    updated_at = CURRENT_TIMESTAMP(3);
