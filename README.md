# gk-invoices-bot

# Configuration

Configuration is taken from `config.json` in the current working directory, or from `GK_INVOICES_CONFIG_FILE`.

Example config

```json
{
  "telegram_token": "<tg token>",
  "storage_dir": "/tmp/gk-inv",
  "nag_interval": "15m0s",
  "email_check_interval": "10m0s",
  "imap_address": "<server>:993",
  "imap_username": "<username>",
  "imap_password": "<password>",
  "notifications_start_time": "08:00",
  "notifications_end_time": "20:00"
}

You can use the telegram token in the `/authorize` command to authorize yourself to use the bot.

