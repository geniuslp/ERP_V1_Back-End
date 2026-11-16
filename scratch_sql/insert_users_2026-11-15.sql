-- Insert 19 users (list rows 1-8 and 15-25; rows 9-14 are skipped on purpose).
-- username = Capitalised first name + '.' + first letter of the surname (lower case), e.g. Kitipat.t
-- department (free text) = the 'position / unit' column; dept_code and roles are NOT set (set them afterwards).
-- password_hash = bcrypt (cost 12) of the agreed initial password; ask everyone to change it after the first login.
-- Rows already present (same username ignoring case, or same employee_code) are skipped, never overwritten.

-- ===== 1) CHECK FIRST (read-only): anything returned here already exists and will be skipped =====
WITH new_users(username, full_name, full_name_th, email, employee_code, department, password_hash) AS (VALUES
  ('Kitipat.t', 'Kitipat Tandhapongse', 'นายกิติพัฒน์ ตัณฑพงศ์', 'kitipat@geniuslp.com', 'GNE-001', 'บริหาร', '$2a$12$wGLGeA6CLPwqUco23HCej.yNBz8.b1tbZMM.rwaQntCCQ0pnq3CRi'),
  ('Sombat.p', 'Sombat Pooudom', 'นายสมบัติ ภูอุดม', 'sombat@geniuslp.com', 'GNE-002', 'บริหาร', '$2a$12$hMSNwjRTQKFKHbZTqk29k.nIU4bZlBcsa4BsofMU2CBSWhryCyEv.'),
  ('Prapas.p', 'Prapas Panato', 'นายประภาศ ปาณะโต', 'prapas@geniuslp.com', 'GNE-005', 'บริหาร', '$2a$12$yjZqlsGhRz0wvCC2HqizTe1A9Iukd81xYbkZTnvFYmr78ewx8/3jC'),
  ('Makorn.p', 'Makorn Pholpermpool', 'นายมกร ผลเพิ่มพูล', 'makorn@geniuslp.com', 'GNE-006', 'บริหาร', '$2a$12$3oFhpAUoZmYz06XyMj/PQu7StziBM4dapGnXFlTXparlwyiC6cNgu'),
  ('Sureeporn.w', 'Sureeporn Wongassadaporn', 'นางสาวสุรีย์พร วงศ์อัษฎาพร', 'sureeporn.w@geniuslp.com', 'GNE-603', 'บริหาร', '$2a$12$3EF8UTys68yTaAkXk7ZiFO2yX4JAsz2.iaNcMFRh.aQijK4mCU.w.'),
  ('Apisada.t', 'Apisada Tandhapongse', 'นางสาวอภิษฎา ตัณฑพงศ์', 'apisada.t@geniuslp.com', 'GNE-410', 'ผู้จัดการฝ่าย', '$2a$12$SSIQXuWI54H5Xb/8UD.D8.WmBHmlTd/s.AIvwX3GA9zMhBhif9qma'),
  ('Sivadon.p', 'Sivadon Pooudom', 'นายศิวดล ภูอุดม', 'sivadon.p@geniuslp.com', 'GNE-619', 'ผู้จัดการฝ่าย', '$2a$12$mf9x7/DDcISNVZht8DTZgucK.jXBpj6CpkmLaE.cxJ.wn0bApahkK'),
  ('Chotiros.t', 'Chotiros Tuntanun', 'นางสาวโชติรส ตัณฑนันท์', 'chotiros@geniuslp.com', 'GNE-047', 'เจ้าหน้าที่อาวุโส', '$2a$12$E4Mmh5pHfYETjFClgVdsCOEhS2CzJgom8NxJTrNA/uOBZ5FD7jFlm'),
  ('Nutthachanun.w', 'Nutthachanun Wanwong', 'นางสาวณัฎฐชานันท์ วันวงษ์', 'nutthachanun.w@geniuslp.com', 'GNE-661', 'ผู้จัดการฝ่าย', '$2a$12$nmMyqMWxJmZ4JANn242dZuinDG6D4cxTfP9r4Ay4WLwug/6Be2Ycm'),
  ('Pichai.c', 'Pichai Chantaviroon', 'นายพิชัย ฉันทวิรุฬห์', 'pichai.ch@geniuslp.com', 'GNE-631', 'ผู้จัดการฝ่าย', '$2a$12$Am5wEtjsfkQlD022qG/VdeBvApzRXLhsF7omb1ZxCWF7hX21I8DxC'),
  ('Pondsnara.s', 'Pondsnara Saowalaksakul', 'นางสาวพรนรา เสาวลักษณ์สกุล', 'pondsnara.s@geniuslp.com', 'GNE-627', 'หัวหน้าส่วน', '$2a$12$JuNjICuJBQJymSgtYY88subp5fr6SPLsBuyiFDf2ewORInCxuSoWm'),
  ('Thanyalak.b', 'Thanyalak Boonsawat', 'นางสาวธัญญลักษ์ บุญสวัสดิ์', 'thanyalak.bo@geniuslp.com', 'GNE-671', 'เจ้าหน้าที่อาวุโส', '$2a$12$9gwuDtDtbMURifqZTJK.2ehLhp2xxlveLrMiuzZFn0PWDTvuPxf7m'),
  ('Channarong.r', 'Channarong Rattanapan', 'นายชาญณรงค์ รัตนพันธ์', 'Channarong.r@geniuslp.com', 'GNE-615', 'เจ้าหน้าที่อาวุโส', '$2a$12$XyVjblLMciRA940AYKeq1ejueS6PwJMAY1X/e..uXUIIEmCHgBQ3G'),
  ('Atchara.p', 'Atchara Pasuwan', 'นางสาวอัจฉรา พาสุวรรณ์', 'atchara.p@geniuslp.com', 'GNE-680', 'เจ้าหน้าที่อาวุโส', '$2a$12$lE16edxGAeTkR5M3gHNKa.2YgjZHZy3vBtuhSqJieOZqrmUPfa0Xq'),
  ('Thitipong.t', 'Thitipong Tapanadilok', 'นายฐิติพงศ์ ฐาปนะดิลก', 'thitipong.t@geniuslp.com', 'GNE444', 'วิศวกร', '$2a$12$eFZw0Ce3EpurbiC1sCEHYu5TQgsGNqfo1ec0Wm4npWO73S1K/5Fa.'),
  ('Weerayooth.k', 'Weerayooth Kewruksa', 'นายวีระยุทธ แก้วรักษา', 'weerayooth.k@geniuslp.com', 'GNE-643', 'เจ้าหน้าที่อาวุโส', '$2a$12$3PjZwY55WAUz/61GAWC6pOd3fKmSUnqwlV/kp3che2pi1QIJcq5mK'),
  ('Sukanya.t', 'Sukanya Thepwinyakit', 'นางสาวสุกัญญา เทพวิญญากิจ', 'sukanya.th@geniuslp.com', 'GNE-677', 'เจ้าหน้าที่อาวุโส', '$2a$12$vhl5zbajaS5m5FF6E4JoEOg21kMlAZqocbo5cNnNalfU0eWwihRBG'),
  ('Arreeya.n', 'Arreeya Nachom', 'นางสาวอารียา น่าชม', 'areerat.ng@geniuslp.com', 'GNE-642', 'เจ้าหน้าที่', '$2a$12$WGlZtD0gl5sW3Q2nH5Th8egMcyOCoFQr15PjTpmj82tnP0PsFcACy'),
  ('Areerat.n', 'Areerat Ngamnak', 'นางสาวอารีรัตน์ งามนัก', 'areerat.ng@geniuslp.com', 'GNE-667', 'เจ้าหน้าที่', '$2a$12$dd92R2CH9YfqoJCnFIJerOf6/Eh1trHfq7gbHaPmsvWZirDFFCMvy')
)
SELECT n.username AS new_username, n.employee_code AS new_code, u.id AS existing_id, u.username AS existing_username, u.employee_code AS existing_code
FROM new_users n
JOIN users u ON lower(u.username) = lower(n.username) OR (u.employee_code IS NOT NULL AND u.employee_code = n.employee_code)
ORDER BY n.username;

-- ===== 2) INSERT (one statement = one transaction; returns the rows that were created) =====
WITH new_users(username, full_name, full_name_th, email, employee_code, department, password_hash) AS (VALUES
  ('Kitipat.t', 'Kitipat Tandhapongse', 'นายกิติพัฒน์ ตัณฑพงศ์', 'kitipat@geniuslp.com', 'GNE-001', 'บริหาร', '$2a$12$wGLGeA6CLPwqUco23HCej.yNBz8.b1tbZMM.rwaQntCCQ0pnq3CRi'),
  ('Sombat.p', 'Sombat Pooudom', 'นายสมบัติ ภูอุดม', 'sombat@geniuslp.com', 'GNE-002', 'บริหาร', '$2a$12$hMSNwjRTQKFKHbZTqk29k.nIU4bZlBcsa4BsofMU2CBSWhryCyEv.'),
  ('Prapas.p', 'Prapas Panato', 'นายประภาศ ปาณะโต', 'prapas@geniuslp.com', 'GNE-005', 'บริหาร', '$2a$12$yjZqlsGhRz0wvCC2HqizTe1A9Iukd81xYbkZTnvFYmr78ewx8/3jC'),
  ('Makorn.p', 'Makorn Pholpermpool', 'นายมกร ผลเพิ่มพูล', 'makorn@geniuslp.com', 'GNE-006', 'บริหาร', '$2a$12$3oFhpAUoZmYz06XyMj/PQu7StziBM4dapGnXFlTXparlwyiC6cNgu'),
  ('Sureeporn.w', 'Sureeporn Wongassadaporn', 'นางสาวสุรีย์พร วงศ์อัษฎาพร', 'sureeporn.w@geniuslp.com', 'GNE-603', 'บริหาร', '$2a$12$3EF8UTys68yTaAkXk7ZiFO2yX4JAsz2.iaNcMFRh.aQijK4mCU.w.'),
  ('Apisada.t', 'Apisada Tandhapongse', 'นางสาวอภิษฎา ตัณฑพงศ์', 'apisada.t@geniuslp.com', 'GNE-410', 'ผู้จัดการฝ่าย', '$2a$12$SSIQXuWI54H5Xb/8UD.D8.WmBHmlTd/s.AIvwX3GA9zMhBhif9qma'),
  ('Sivadon.p', 'Sivadon Pooudom', 'นายศิวดล ภูอุดม', 'sivadon.p@geniuslp.com', 'GNE-619', 'ผู้จัดการฝ่าย', '$2a$12$mf9x7/DDcISNVZht8DTZgucK.jXBpj6CpkmLaE.cxJ.wn0bApahkK'),
  ('Chotiros.t', 'Chotiros Tuntanun', 'นางสาวโชติรส ตัณฑนันท์', 'chotiros@geniuslp.com', 'GNE-047', 'เจ้าหน้าที่อาวุโส', '$2a$12$E4Mmh5pHfYETjFClgVdsCOEhS2CzJgom8NxJTrNA/uOBZ5FD7jFlm'),
  ('Nutthachanun.w', 'Nutthachanun Wanwong', 'นางสาวณัฎฐชานันท์ วันวงษ์', 'nutthachanun.w@geniuslp.com', 'GNE-661', 'ผู้จัดการฝ่าย', '$2a$12$nmMyqMWxJmZ4JANn242dZuinDG6D4cxTfP9r4Ay4WLwug/6Be2Ycm'),
  ('Pichai.c', 'Pichai Chantaviroon', 'นายพิชัย ฉันทวิรุฬห์', 'pichai.ch@geniuslp.com', 'GNE-631', 'ผู้จัดการฝ่าย', '$2a$12$Am5wEtjsfkQlD022qG/VdeBvApzRXLhsF7omb1ZxCWF7hX21I8DxC'),
  ('Pondsnara.s', 'Pondsnara Saowalaksakul', 'นางสาวพรนรา เสาวลักษณ์สกุล', 'pondsnara.s@geniuslp.com', 'GNE-627', 'หัวหน้าส่วน', '$2a$12$JuNjICuJBQJymSgtYY88subp5fr6SPLsBuyiFDf2ewORInCxuSoWm'),
  ('Thanyalak.b', 'Thanyalak Boonsawat', 'นางสาวธัญญลักษ์ บุญสวัสดิ์', 'thanyalak.bo@geniuslp.com', 'GNE-671', 'เจ้าหน้าที่อาวุโส', '$2a$12$9gwuDtDtbMURifqZTJK.2ehLhp2xxlveLrMiuzZFn0PWDTvuPxf7m'),
  ('Channarong.r', 'Channarong Rattanapan', 'นายชาญณรงค์ รัตนพันธ์', 'Channarong.r@geniuslp.com', 'GNE-615', 'เจ้าหน้าที่อาวุโส', '$2a$12$XyVjblLMciRA940AYKeq1ejueS6PwJMAY1X/e..uXUIIEmCHgBQ3G'),
  ('Atchara.p', 'Atchara Pasuwan', 'นางสาวอัจฉรา พาสุวรรณ์', 'atchara.p@geniuslp.com', 'GNE-680', 'เจ้าหน้าที่อาวุโส', '$2a$12$lE16edxGAeTkR5M3gHNKa.2YgjZHZy3vBtuhSqJieOZqrmUPfa0Xq'),
  ('Thitipong.t', 'Thitipong Tapanadilok', 'นายฐิติพงศ์ ฐาปนะดิลก', 'thitipong.t@geniuslp.com', 'GNE444', 'วิศวกร', '$2a$12$eFZw0Ce3EpurbiC1sCEHYu5TQgsGNqfo1ec0Wm4npWO73S1K/5Fa.'),
  ('Weerayooth.k', 'Weerayooth Kewruksa', 'นายวีระยุทธ แก้วรักษา', 'weerayooth.k@geniuslp.com', 'GNE-643', 'เจ้าหน้าที่อาวุโส', '$2a$12$3PjZwY55WAUz/61GAWC6pOd3fKmSUnqwlV/kp3che2pi1QIJcq5mK'),
  ('Sukanya.t', 'Sukanya Thepwinyakit', 'นางสาวสุกัญญา เทพวิญญากิจ', 'sukanya.th@geniuslp.com', 'GNE-677', 'เจ้าหน้าที่อาวุโส', '$2a$12$vhl5zbajaS5m5FF6E4JoEOg21kMlAZqocbo5cNnNalfU0eWwihRBG'),
  ('Arreeya.n', 'Arreeya Nachom', 'นางสาวอารียา น่าชม', 'areerat.ng@geniuslp.com', 'GNE-642', 'เจ้าหน้าที่', '$2a$12$WGlZtD0gl5sW3Q2nH5Th8egMcyOCoFQr15PjTpmj82tnP0PsFcACy'),
  ('Areerat.n', 'Areerat Ngamnak', 'นางสาวอารีรัตน์ งามนัก', 'areerat.ng@geniuslp.com', 'GNE-667', 'เจ้าหน้าที่', '$2a$12$dd92R2CH9YfqoJCnFIJerOf6/Eh1trHfq7gbHaPmsvWZirDFFCMvy')
)
INSERT INTO users (username, password_hash, full_name, full_name_th, email, employee_code, department, is_active, created_at, updated_at)
SELECT n.username, n.password_hash, n.full_name, n.full_name_th, n.email, n.employee_code, n.department, true, NOW(), NOW()
FROM new_users n
WHERE NOT EXISTS (
  SELECT 1 FROM users u
  WHERE lower(u.username) = lower(n.username)
     OR (u.employee_code IS NOT NULL AND u.employee_code = n.employee_code)
)
RETURNING id, username, employee_code, full_name;

-- ===== 3) VERIFY: should list the new users, each with is_active = true and no dept_code / roles yet =====
SELECT id, username, employee_code, full_name, full_name_th, department, email, is_active, dept_code
FROM users
WHERE employee_code IN ('GNE-001','GNE-002','GNE-005','GNE-006','GNE-603','GNE-410','GNE-619','GNE-047','GNE-661','GNE-631','GNE-627','GNE-671','GNE-615','GNE-680','GNE444','GNE-643','GNE-677','GNE-642','GNE-667')
ORDER BY employee_code;
