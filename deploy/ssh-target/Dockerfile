# Minimal SSH target for Agent Hub e2e (password auth).
FROM alpine:3.20

RUN apk add --no-cache openssh openssh-server bash \
  && ssh-keygen -A \
  && echo "root:targetpass" | chpasswd \
  && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config \
  && sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
  && sed -i 's/^#\?PermitEmptyPasswords.*/PermitEmptyPasswords no/' /etc/ssh/sshd_config \
  && printf '\nUsePAM no\n' >> /etc/ssh/sshd_config

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
