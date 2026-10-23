FROM python:3.11-slim

RUN useradd --uid 10001 --no-create-home app
WORKDIR /app
COPY pyproject.toml ./
COPY src ./src
RUN pip install --no-cache-dir .

USER app
EXPOSE 5020 4840
ENV PYTHONUNBUFFERED=1
CMD ["python", "-m", "plantorq_simulator"]
