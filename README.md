# Sistema de Temperatura por CEP

Este projeto consiste em dois serviços desenvolvidos em Go:

- **Serviço A**: Responsável por receber CEPs através de uma API REST
- **Serviço B**: Orquestra a busca de CEP e consulta da temperatura

## Tecnologias Utilizadas

- Go 1.23+
- Docker e Docker Compose
- OpenTelemetry para tracing distribuído
- Zipkin para visualização de traces
- APIs externas:
  - ViaCEP
  - WeatherAPI

## Pré-requisitos

- Docker e Docker Compose instalados
- Chave de API da WeatherAPI (obtenha em https://www.weatherapi.com/)

## Como executar o projeto

1. Clone o repositório:
   ```bash
   git clone <url-do-repositorio>
   cd <nome-do-repositorio>
   ```

2. Crie um arquivo `.env` na raiz do projeto para armazenar sua chave de API:
   ```
   WEATHER_API_KEY=sua_chave_da_weatherapi_aqui
   ```

3. Execute o projeto usando Docker Compose:
   ```bash
   docker-compose up --build
   ```

4. Os serviços estarão disponíveis nas seguintes URLs:
   - Serviço A: http://localhost:8080/zipcode
   - Serviço B: http://localhost:8081/weather
   - Zipkin (para visualização dos traces): http://localhost:9411

## Exemplo de Uso

Para testar o sistema, envie uma requisição POST para o Serviço A:

```bash
curl -X POST http://localhost:8080/zipcode -H "Content-Type: application/json" -d '{"cep":"01001000"}'
```

A resposta deve ser algo como:

```json
{
  "city": "São Paulo",
  "temp_C": 28.5,
  "temp_F": 83.3,
  "temp_K": 301.65
}
```

## Estrutura do projeto

```
.
├── docker-compose.yml
├── README.md
├── service-a
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── main.go
├── service-b
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── main.go
└── otel-collector-config.yaml
```

## Observabilidade e Tracing

O projeto utiliza OpenTelemetry para instrumentação e Zipkin para visualização dos traces. Foram implementados spans para medir:

1. Tempo de resposta do serviço de busca de CEP
2. Tempo de resposta do serviço de busca de temperatura
3. Tempo total de processamento de cada requisição

Para visualizar os traces, acesse o Zipkin através da URL http://localhost:9411.

## Detalhes técnicos

### Serviço A

- Recebe um CEP via POST
- Valida se o CEP tem 8 dígitos e é uma string
- Encaminha o CEP para o Serviço B
- Retorna a resposta do Serviço B ou mensagens de erro adequadas

### Serviço B

- Recebe um CEP do Serviço A
- Utiliza a API ViaCEP para obter a cidade correspondente
- Utiliza a API WeatherAPI para obter a temperatura atual da cidade
- Realiza as conversões de temperatura (Celsius → Fahrenheit, Celsius → Kelvin)
- Retorna os dados formatados ou mensagens de erro adequadas

### Respostas de erro

- 422: "invalid zipcode" - quando o CEP não segue o formato correto
- 404: "can not find zipcode" - quando o CEP não é encontrado
- 500: erro interno do servidor