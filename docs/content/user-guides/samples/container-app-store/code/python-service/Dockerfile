FROM python:3.9-slim
# RUN apk add g++
COPY requirements.txt /app/
WORKDIR /app
RUN pip install -r requirements.txt
COPY . .
ENTRYPOINT ["python"]
EXPOSE 5000
CMD ["app.py"]