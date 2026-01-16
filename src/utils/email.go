package utils

import (
	"fmt"
	"net/smtp"
	"regexp"

	"tiger.com/v2/src/config"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func IsValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

func SendEmail(cfg *config.Config, to []string, subject, body string) error {
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPHost)

	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s\r\n", to[0], subject, body))

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	return smtp.SendMail(addr, auth, cfg.SMTPFrom, to, msg)
}

func SendRegisterOTP(cfg *config.Config, to string, otp string) error {
	subject := "Xác thực tài khoản Tiger Esport"

	body := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0; padding:0; background:#f8f9fa; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif; color:#202124; font-size:14px; line-height:1.5;">
  
  <table width="100%%" cellpadding="0" cellspacing="0" border="0" style="background:#f8f9fa; padding:20px;">
    <tr>
      <td align="center">
        
        <table width="600" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff; border-radius:3px; overflow:hidden;">
          
          <tr>
            <td style="padding:20px;">
              
              <h2 style="margin:0 0 20px; font-size:20px; font-weight:bold;">
                Xác thực tài khoản Tiger Esport
              </h2>
              
              <p style="margin:0 0 20px;">
                Chào mừng bạn đến với Tiger Esport! Để hoàn tất đăng ký tài khoản, vui lòng nhập mã xác thực bên dưới vào ứng dụng.
              </p>

              <div style="text-align:center; margin:20px 0; padding:20px; background:#f8f9fa; border-radius:3px;">
                <p style="margin:0 0 10px; font-size:12px; text-transform:uppercase; font-weight:bold; color:#5f6368;">
                  Mã xác thực
                </p>
                <div style="font-size:32px; font-weight:bold; letter-spacing:5px;">
                  %s
                </div>
                <p style="margin:10px 0 0; font-size:12px; color:#5f6368;">
                  Mã này hết hạn sau <strong>5 phút</strong>
                </p>
              </div>

              <p style="margin:20px 0 0;">
                Nếu bạn không yêu cầu mã xác thực này, vui lòng bỏ qua email này. Tài khoản của bạn vẫn an toàn.
              </p>

              <div style="margin-top:20px; padding:15px; background:#f8f9fa; border-radius:3px;">
                <p style="margin:0 0 10px; font-weight:bold; color:#5f6368;">
                  Lưu ý bảo mật
                </p>
                <ul style="margin:0; padding:0 0 0 20px; list-style-type:disc; color:#5f6368; font-size:13px;">
                  <li style="margin-bottom:5px;">Không chia sẻ mã này với bất kỳ ai, kể cả nhân viên Tiger Esport</li>
                  <li style="margin-bottom:5px;">Tiger Esport không bao giờ yêu cầu mã xác thực qua điện thoại hoặc tin nhắn</li>
                  <li>Luôn kiểm tra kỹ địa chỉ email người gửi trước khi nhập mã</li>
                </ul>
              </div>

            </td>
          </tr>

          <tr>
            <td style="padding:20px; background:#f8f9fa; text-align:center; font-size:12px; color:#5f6368;">
              Đây là email tự động, vui lòng không trả lời.<br>
              © 2025 Tiger Esport. Bảo lưu mọi quyền.
            </td>
          </tr>

        </table>

      </td>
    </tr>
  </table>

</body>
</html>
`, otp)

	return SendEmail(cfg, []string{to}, subject, body)
}

func SendResetPasswordOTP(cfg *config.Config, to string, otp string) error {
	subject := "Đặt lại mật khẩu Tiger Esport của bạn"

	body := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0; padding:0; background:#f8f9fa; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif; color:#202124; font-size:14px; line-height:1.5;">
  
  <table width="100%%" cellpadding="0" cellspacing="0" border="0" style="background:#f8f9fa; padding:20px;">
    <tr>
      <td align="center">
        
        <table width="600" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff; border-radius:3px; overflow:hidden;">
          
          <tr>
            <td style="padding:20px;">
              
              <h2 style="margin:0 0 20px; font-size:20px; font-weight:bold;">
                Đặt lại mật khẩu Tiger Esport
              </h2>
              
              <p style="margin:0 0 20px;">
                Chúng tôi đã nhận được yêu cầu đặt lại mật khẩu cho tài khoản Tiger Esport của bạn. Sử dụng mã xác thực bên dưới để tiếp tục.
              </p>

              <div style="text-align:center; margin:20px 0; padding:20px; background:#f8f9fa; border-radius:3px;">
                <p style="margin:0 0 10px; font-size:12px; text-transform:uppercase; font-weight:bold; color:#5f6368;">
                  Mã xác thực
                </p>
                <div style="font-size:32px; font-weight:bold; letter-spacing:5px;">
                  %s
                </div>
                <p style="margin:10px 0 0; font-size:12px; color:#5f6368;">
                  Mã này hết hạn sau <strong>5 phút</strong>
                </p>
              </div>

              <p style="margin:20px 0 0;">
                Nếu bạn không yêu cầu đặt lại mật khẩu, vui lòng bỏ qua email này. Mật khẩu của bạn sẽ không thay đổi.
              </p>

              <div style="margin-top:20px; padding:15px; background:#f8f9fa; border-radius:3px;">
                <p style="margin:0 0 10px; font-weight:bold; color:#5f6368;">
                  Lưu ý bảo mật
                </p>
                <ul style="margin:0; padding:0 0 0 20px; list-style-type:disc; color:#5f6368; font-size:13px;">
                  <li style="margin-bottom:5px;">Sử dụng mật khẩu mạnh và duy nhất</li>
                  <li style="margin-bottom:5px;">Không chia sẻ mã xác thực</li>
                  <li>Bật xác thực hai yếu tố</li>
                </ul>
              </div>

            </td>
          </tr>

          <tr>
            <td style="padding:20px; background:#f8f9fa; text-align:center; font-size:12px; color:#5f6368;">
              Đây là email tự động, vui lòng không trả lời.<br>
              © 2025 Tiger Esport. Bảo lưu mọi quyền.
            </td>
          </tr>

        </table>

      </td>
    </tr>
  </table>

</body>
</html>
`, otp)

	return SendEmail(cfg, []string{to}, subject, body)
}

func ChangePasswordNotify(cfg *config.Config, to string) error {
	subject := "Mật khẩu Tiger Esport của bạn đã được thay đổi"

	body := `
<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0; padding:0; background:#f8f9fa; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif; color:#202124; font-size:14px; line-height:1.5;">
  
  <table width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f8f9fa; padding:20px;">
    <tr>
      <td align="center">
        
        <table width="600" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff; border-radius:3px; overflow:hidden;">
          
          <tr>
            <td style="padding:20px;">
              
              <h2 style="margin:0 0 20px; font-size:20px; font-weight:bold;">
                Mật khẩu Tiger Esport đã được thay đổi
              </h2>
              
              <p style="margin:0 0 20px;">
                Đây là xác nhận rằng mật khẩu cho tài khoản Tiger Esport của bạn đã được thay đổi thành công. Tài khoản của bạn giờ đây được bảo mật với mật khẩu mới.
              </p>

              <p style="margin:20px 0 0;">
                Nếu bạn không thực hiện thay đổi này, vui lòng đặt lại mật khẩu ngay lập tức và liên hệ hỗ trợ.
              </p>

              <div style="margin-top:20px; padding:15px; background:#f8f9fa; border-radius:3px;">
                <p style="margin:0 0 10px; font-weight:bold; color:#5f6368;">
                  Mẹo bảo mật
                </p>
                <ul style="margin:0; padding:0 0 0 20px; list-style-type:disc; color:#5f6368; font-size:13px;">
                  <li style="margin-bottom:5px;">Sử dụng mật khẩu duy nhất cho Tiger Esport</li>
                  <li style="margin-bottom:5px;">Bật xác thực hai yếu tố</li>
                  <li style="margin-bottom:5px;">Không chia sẻ mật khẩu</li>
                  <li>Thay đổi mật khẩu định kỳ</li>
                </ul>
              </div>

            </td>
          </tr>

          <tr>
            <td style="padding:20px; background:#f8f9fa; text-align:center; font-size:12px; color:#5f6368;">
              Đây là email tự động, vui lòng không trả lời.<br>
              © 2025 Tiger Esport. Bảo lưu mọi quyền.
            </td>
          </tr>

        </table>

      </td>
    </tr>
  </table>

</body>
</html>
`

	return SendEmail(cfg, []string{to}, subject, body)
}
