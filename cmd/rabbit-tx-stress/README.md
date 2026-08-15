# Rabbit Chain Transaction Stress Auditor

Auditor exclusivo do laboratório `/tmp/rabbit-20nodes`. Ele assina localmente
transações EIP-1559 usando a conta líquida de teste do node20 e executa cinco
lotes de 25 transferências por padrão.

O relatório valida:

- 125 nonces consecutivos;
- receipts e status de todas as transações;
- valor, taxa, tip e base fee queimada;
- deltas de saldo do remetente, destinatários e produtores;
- blocos canônicos em todos os 20 nós;
- txpool vazio em todos os nós ao final;
- rejeição de transação duplicada, nonce antigo, saldo insuficiente e chain ID
  incorreto.

O programa não altera o genesis e não utiliza recompensas bloqueadas. O saldo
líquido usado é o saldo temporário existente somente no genesis runtime do
laboratório.
