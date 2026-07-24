# Minimal SSH target for Agent Hub e2e (password auth).
# Listens on uncommon port 27343 to avoid Coolify / host SSH clashes on 22.
FROM alpine:3.20

RUN apk add --no-cache openssh openssh-server bash tmux \
  && ssh-keygen -A \
  && echo "root:targetpass" | chpasswd \
  && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config \
  && sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
  && sed -i 's/^#\?PermitEmptyPasswords.*/PermitEmptyPasswords no/' /etc/ssh/sshd_config \
  && sed -i 's/^#\?Port .*/Port 27343/' /etc/ssh/sshd_config \
  && grep -q '^Port 27343' /etc/ssh/sshd_config || echo 'Port 27343' >> /etc/ssh/sshd_config \
  && printf '\nUsePAM no\n' >> /etc/ssh/sshd_config

EXPOSE 27343
CMD ["/usr/sbin/sshd", "-D", "-e", "-p", "27343"]
